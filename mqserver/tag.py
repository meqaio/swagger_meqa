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

class Definition(object):
    def __init__(self, name, swagger):
        self.name = name
        self.norm_name = swagger.vocab.normalize(name)
        orig_properties = swagger.get_properties(swagger.doc['definitions'][name])
        self.properties = dict()

        for orig_prop in orig_properties:
            norm_prop = swagger.vocab.normalize(orig_prop)
            self.properties[norm_prop] = orig_prop

class SwaggerDoc(object):
    MethodGet = "get"
    MethodPut = "put"
    MethodPost = "post"
    MethodDelete = "delete"
    MethodHead = "head"
    MethodPatch = "patch"
    MethodOptions = "options"
    MethodAll = [MethodGet, MethodPut, MethodPost, MethodDelete, MethodHead, MethodPatch, MethodOptions]

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
    def iterate_schema(self, schema, callback):
        if 'allOf' in schema:
            for s in schema['allOf']:
                self.iterate_schema(s, callback)
            return

        if '$ref' in schema:
            refstr = schema['$ref']
            # we only handle local refs for now
            if refstr[0] != '#':
                return

            reflist = refstr.split('/')
            refschema = self.doc[reflist[1]][reflist[2]]
            return self.iterate_schema(refschema, callback)

        return callback(schema)

    # return the object, property name that matches with the phrases
    def find_obj_property(self, phrase, property_phrase):
        min_cost = len(phrase)
        min_obj = None
        for obj_name, obj in self.definitions.items():
            cost = match_phrase(phrase, obj_name)
            if cost < 0:
                continue

            if cost < min_cost:
                min_cost = cost
                min_obj = obj
                if min_cost == 0:
                    break

        if min_obj == None:
            return '', ''

        if property_phrase == '':
            property_phrase = phrase

        min_cost = len(property_phrase)
        min_property = None
        for prop_norm_name, prop_orig_name in min_obj.properties.items():
            if property_phrase == prop_orig_name or property_phrase == prop_norm_name:
                cost = 0
            else:
                cost = match_phrase(property_phrase, prop_norm_name)
                if cost < 0:
                    continue

            if cost < min_cost:
                min_cost = cost
                min_property = prop_orig_name
                if min_cost == 0:
                    break

        if min_property == None:
            return '', ''

        return min_obj.name, min_property

    # given path string, method string and the parameter dict, try to insert a tag into the param
    def guess_tag(self, path, method, param):
        desc = param.get('description')
        tag = get_meqa_tag(desc)
        if tag != None:
            # a tag exist already, skip
            return

        if param.get('enum') != None or param.get('schema') != None:
            return

        if desc == None:
            desc = ''

        # we try to find a class.property pair such that the normalized name of class and property appear
        # in the name of the property. If we can't do this, we try throw in the description field.
        # TODO, the description field can be big, we should do some syntactic analysis to it to trim it down.
        def match_to_name(phrase, property_phrase):
            objname, propname = self.find_obj_property(phrase, property_phrase)
            if objname != '' and propname != '':
                param['description'] = desc + ' ' + MeqaTag(objname, propname, '', 0).to_string()
                return True
            return False

        param_name = param.get('name')
        if param.get('in') == 'path':
            # the word right before the parameter usually is the class name. We prefer this. Note that in this
            # case the property_name should just be the param_name, not the normalized name.
            index = path.find(param_name)
            if index > 0:
                norm_path = self.vocab.normalize(path[:index - 2])
                found = match_to_name(norm_path, param_name)
                if found:
                    return

        norm_name = self.vocab.normalize(param_name)
        found = match_to_name(norm_name, '')
        if found:
            return

        norm_desc = self.vocab.normalize(desc)
        found = match_to_name(norm_desc, '')
        if found:
            return

        match_to_name(norm_name + norm_desc, '')

    def add_tags(self):
        # try to create tags and add them to the param's description field
        def create_tags_for_param(params):
            if params == None:
                return
            for p in params:
                self.guess_tag(pathname, method, p)

        paths = self.doc['paths']
        for pathname, path in paths.items():
            method = ''
            create_tags_for_param(path.get('parameters'))
            for method in SwaggerDoc.MethodAll:
                if method in path:
                    create_tags_for_param(path.get(method).get('parameters'))

    def get_properties(self, schema):
        properties = set()
        def add_properties(schema):
            nonlocal properties
            prop = set()
            if 'properties' in schema:
                for pname in schema['properties']:
                    prop.add(pname)
            properties = properties.union(prop)

        self.iterate_schema(schema, add_properties)
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

        for prop_norm_name, prop_orig_name in obj.properties.items():
            print("{} -> {}\n".format(prop_norm_name, prop_orig_name))

if __name__ == '__main__':
    main()