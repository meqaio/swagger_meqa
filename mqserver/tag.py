import pdb
import argparse
import json
import logging
import re
import string
from vocabulary import Vocabulary

from ruamel.yaml import YAML
from pathlib import Path
from math import log

def load_yaml(filename):
    yaml = YAML()
    yamlPath = Path(filename)
    if not yamlPath.exists():
        return None
    doc = yaml.load(yamlPath)
    return doc

class MeqaTag(object):
    FlagSuccess = 1
    FlagFail = 2
    FlagWeak = 4

    def __init__(self, cl, pr, op, fl):
        self.classname = cl
        self.property = pr
        self.operation = op
        self.flags = fl

    def to_string(self):
        str = '<meqa ' + self.classname
        if len(self.property) > 0:
            str += '.' + self.property
        if len(self.operation) > 0:
            str += '.' + self.operation
        str += '>'
        return str

# get the meqa tag from the string desc
def get_meqa_tag(desc):
    if desc == None:
        return None

    found = re.search('<meqa *[/-~\\-]+\\.?[/-~\\-]*\\.?[a-zA-Z]* *[a-zA-Z,]* *>', desc)
    if found == None:
        return None

    meqa = found.group(0)
    meqa = meqa[6:-1] # remove <meqa and >
    tags = meqa.split(' ')
    flags = 0
    for t in tags:
        if len(t) > 0:
            if t == 'success':
                flags |= MeqaTag.FlagSuccess
            elif t == 'fail':
                flags |= MeqaTag.FlagFail
            elif t == 'weak':
                flags |= MeqaTag.FlagWeak
            else:
                objtags = t


    contents = objtags.split(".")
    lencontents = len(contents)
    if lencontents == 1:
        return MeqaTag(contents[0], '', '', flags)
    elif lencontents == 2:
        return MeqaTag(contents[0], contents[1], '', flags)
    elif lencontents == 3:
        return MeqaTag(contents[0], contents[1], contents[2], flags)
    else:
        return None

# given a long phrase and a short phrase (both normalized), try to find the best match.
# returns -1 if not found, 0 if exact match, distance between words if non-exact match.
def match_phrase(phrase, key):
    phrase_list = phrase.split(' ')
    key_list = key.split(' ')
    if not set(key_list) <= set(phrase_list):
        return -1

    if phrase.find(key) >= 0:
        return 0

    key_index_list = []
    for p in phrase_list:
        found = False
        for i, k in enumerate(key_list):
            if p == k:
                found = True
                key_index_list.append(i)
                break
        if not found:
            key_index_list.append(-1)

    # the key_index_list is the length of the phrase with the key's index. Find the smallest substring
    # that has all the keys. The observation is that the smallest span will always only have one of
    # each key.
    min_span = len(key_index_list)
    last_key = dict() # key index to last key position mapping
    for i, k in enumerate(key_index_list):
        if k > 0:
            last_key[k] = i
            if len(last_key) < len(key_list):
                continue
            # note that only the most recent information matters
            min_span = min(min_span, max(last_key.values()) - min(last_key.values()))
    return min_span

class Property(object):
    def __init__(self, name, type, swagger):
        self.name = name
        self.norm_name = swagger.vocab.normalize(name)
        self.type = type

class Definition(object):
    def __init__(self, name, swagger):
        self.name = name
        self.norm_name = swagger.vocab.normalize(name)
        self.properties = swagger.get_properties(swagger.doc['definitions'][name])

class SwaggerDoc(object):
    MethodGet = "get"
    MethodPut = "put"
    MethodPost = "post"
    MethodDelete = "delete"
    MethodHead = "head"
    MethodPatch = "patch"
    MethodOptions = "options"
    MethodAll = [MethodGet, MethodPut, MethodPost, MethodDelete, MethodHead, MethodPatch, MethodOptions]

    TypeBoolean = 'boolean'
    TypeInteger = 'integer'
    TypeNumber = 'number'
    TypeObject = 'object'
    TypeArray = 'array'
    TypeString = 'string'

    def __init__(self, filename):
        self.vocab = Vocabulary()
        self.doc = load_yaml(filename)
        self.definitions = dict()  # holds the object definitions.
        self.logger = logging.getLogger(name='meqa')

    def dump(self, filename):
        yaml = YAML()
        f = open(filename, 'w')
        yaml.dump(self.doc, f)
        f.close()

    # Iterate through the schema. Call the callback for each bottom level schema (object or array) we discover.
    # The path is the list of keys we can use to traverse to the object from the swagger doc root. e.g.
    # [definitions, Pet, id]
    def iterate_schema(self, schema, callback, path, follow_array=False, follow_ref=True, follow_object=False):
        callback(schema, path)
        if 'allOf' in schema:
            for s in schema['allOf']:
                self.iterate_schema(s, callback, path, follow_array, follow_ref, follow_object)
            return

        if '$ref' in schema:
            if not follow_ref:
                return

            refstr = schema['$ref']
            # we only handle local refs for now
            if refstr[0] != '#':
                return

            reflist = refstr.split('/')
            refschema = self.doc[reflist[1]][reflist[2]]
            self.iterate_schema(refschema, callback, reflist[1:3], follow_array, follow_ref, follow_object)
            return

        if schema.get('type') == 'array':
            if not follow_array:
                return

            item_schema = schema.get('items')
            return self.iterate_schema(item_schema, callback, path, follow_array, follow_ref, follow_object)

        if schema.get('type') == 'object':
            if not follow_object:
                return
            properties = schema.get('properties')
            if properties == None:
                return
            for k, v in properties.items():
                path.append(k)
                self.iterate_schema(v, callback, path, follow_array, follow_ref, follow_object)
                path = path[:-1]

    # return the object, property name that matches with the phrases. If the passed in property_type is None
    # we don't try to match against property.
    def find_obj_property(self, phrase, property_phrase, property_type, exclude=None):
        if property_phrase == '':
            property_phrase = phrase
        min_cost = len(phrase) + len(property_phrase)
        min_obj = None

        for obj_name, obj in self.definitions.items():
            if obj.name == exclude:
                continue

            cost = match_phrase(phrase, obj_name)
            if cost < 0:
                continue

            min_property_cost = len(property_phrase)
            min_property = None
            for prop in obj.properties:
                if property_type == None:
                    # we are ok except for object type. We only allow matching against object type
                    # if the caller specifies it.
                    if prop.type == SwaggerDoc.TypeObject:
                        continue
                elif property_type != prop.type:
                    # type doesn't match. We still allow matching integer and number types.
                    if property_type != SwaggerDoc.TypeInteger and property_type != SwaggerDoc.TypeNumber or \
                        prop.type != SwaggerDoc.TypeInteger and prop.type != SwaggerDoc.TypeNumber:
                        continue

                property_cost = 0
                if property_phrase != prop.name and property_phrase != prop.norm_name:
                    property_cost = match_phrase(property_phrase, prop.norm_name)
                    if property_cost < 0:
                        continue

                if property_cost < min_property_cost:
                    min_property_cost = property_cost
                    min_property = prop.name
                    if min_property_cost == 0:
                        break

            if min_property == None:
                continue

            cost += min_property_cost
            if cost < min_cost:
                min_cost = cost
                min_obj = obj
                if min_cost == 0:
                    break

        if min_obj == None or min_property == None:
            return '', '', -1

        return min_obj.name, min_property, min_cost

    def add_tag(self, objname, propname, param):
        desc = param.get('description')
        if desc == None:
            desc = ''
        else:
            desc += ' '
        param['description'] = desc + MeqaTag(objname, propname, '', 0).to_string()

    # find the class.property, and if found, add the meqa tag to param. return found or not
    def try_add_tag(self, phrase, property_phrase, param, param_type_match, exclude=None):
        objname, propname, cost = self.find_obj_property(phrase, property_phrase, param_type_match, exclude)
        if objname != '' and propname != '':
            self.add_tag(objname, propname, param)
            return True
        return False

    def should_try_tag(self, param):
        desc = param.get('description')
        tag = get_meqa_tag(desc)
        if tag != None:
            # a tag exist already, skip
            return False

        if param.get('enum') != None:
            return False
        return True

    # given path string, method string and the parameter dict, try to insert a tag into the param
    def guess_tag_for_param(self, path, method, param):
        if not self.should_try_tag(param):
            return

        desc = param.get('description')
        if desc == None:
            desc = ''

        # we try to find a class.property pair such that the normalized name of class and property appear
        # in the name of the property. If we can't do this, we try throw in the description field.
        # TODO, the description field can be big, we should do some syntactic analysis to it to trim it down.

        param_name = param.get('name')
        param_in = param.get('in')
        if param_in == 'path':
            param_type_match = None
        else:
            param_type_match = param.get('type')

        if param_in == 'path':
            # the word right before the parameter usually is the class name. We prefer this. Note that in this
            # case the property_name should just be the param_name, not the normalized name.
            index = path.find(param_name)
            if index > 0:
                norm_path = self.vocab.normalize(path[:index - 2])
                found = self.try_add_tag(norm_path, param_name, param, param_type_match)
                if found:
                    return

        # if body has schema, we should iterate down into the schema, instead of just look at
        # the param object.
        param_schema = param.get('schema')
        if param_schema != None:
            return self.guess_tag_for_schema(param_schema, [param_name], None)

        norm_name = self.vocab.normalize(param_name)
        found = self.try_add_tag(norm_name, '', param, param_type_match)
        if found:
            return

        # One sentence at a time.
        sentences = desc.split('.')
        min_cost = 1e99
        for sentence in sentences:
            norm_desc = self.vocab.normalize(sentence)
            objname, propname, cost = self.find_obj_property(norm_desc, '', param_type_match)
            if cost < 0:
                continue

            if cost < min_cost:
                min_cost = cost
                min_obj = objname
                min_prop = propname

        if min_cost != 1e99:
            self.add_tag(min_obj, min_prop, param)
            return

        # a desparate last effort. Maybe we shouldn't do this.
        self.try_add_tag(norm_name + norm_desc, '', param, param_type_match)

    # the path is the
    def guess_tag_for_schema(self, schema, path, exclude):
        # go through the object properties and try to add tags. We have to be more careful about this
        # one since mistakes will lead to cycles in the dependency graph.
        def add_tag_callback(s, path):
            if not self.should_try_tag(s):
                return

            schema_type = s.get('type')
            if schema_type == SwaggerDoc.TypeInteger or schema_type == SwaggerDoc.TypeNumber or schema_type == SwaggerDoc.TypeString:
                norm_name = self.vocab.normalize(path[-1])
                found = self.try_add_tag(norm_name, '', s, schema_type, exclude)
                if found:
                    return

                # desc = schema.get('description')
                # if desc != None:
                #     self.try_add_tag(self.vocab.normalize(desc), '', schema)

        self.iterate_schema(schema, add_tag_callback, path, follow_array=True, follow_ref=False,
                            follow_object=True)

    def add_tags(self):
        # try to create tags and add them to the param's description field
        def create_tags_for_param(params):
            if params == None:
                return
            for p in params:
                self.guess_tag_for_param(pathname, method, p)

        paths = self.doc['paths']
        for pathname, path in paths.items():
            method = ''
            create_tags_for_param(path.get('parameters'))
            for method in SwaggerDoc.MethodAll:
                if method in path:
                    create_tags_for_param(path.get(method).get('parameters'))

        for defname, schema in self.doc['definitions'].items():
            self.guess_tag_for_schema(schema, ['definitions', defname], defname)

    def get_properties(self, schema):
        properties = []
        def add_properties(schema, path):
            if 'properties' in schema:
                for pname, prop in schema['properties'].items():
                    properties.append(Property(pname, prop.get('type'), self))

        self.iterate_schema(schema, add_properties, [], follow_array=False, follow_ref=True, follow_object=False)
        return properties

    def gather_words(self):
        # we have to do two passes. First time we add all the words into the vocabulary. The second
        # pass we may use the new vocabulary to break down some words that may not be breakable before.
        for name, obj in self.doc['definitions'].items():
            self.vocab.normalize(name)
            if 'properties' in obj:
                for key in obj['properties']:
                    self.vocab.normalize(key)

        # second pass, we lemmarize the unit words and use them as key
        for name in self.doc['definitions']:
            d = Definition(name, self)
            self.definitions[d.norm_name] = d

def main():
    logger = logging.getLogger(name='meqa')
    parser = argparse.ArgumentParser(description='generate tag.yaml from swagger.yaml')
    parser.add_argument("-i", "--input", help="the swagger.yaml file", default="./meqa_data/swagger.yaml")
    parser.add_argument("-o", "--output", help="the output file name", default="./meqa_data/swagger_tagged.yaml")
    args = parser.parse_args()

    swagger = SwaggerDoc(args.input)
    if swagger.doc == None:
        logger.error("Failed to load file %s", args.input)
        return

    print("loaded ", args.input)
    swagger.gather_words()
    print("words gathered")
    swagger.add_tags()
    swagger.dump(args.output)
    print("tags added")

    for norm_name, obj in swagger.definitions.items():
        print("\n======== {} -> {} =========\n".format(norm_name, obj.name))
        print()

        for prop in obj.properties:
            print("{} -> {}\n".format(prop.norm_name, prop.name))

if __name__ == '__main__':
    main()