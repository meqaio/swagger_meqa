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


class Definition(object):
    def __init__(self, name, swagger):
        self.name = name
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

    # given path string, method string and the parameter dict, try to insert a tag into the param
    def guess_tag(self, path, method, param):
        desc = param.get('description')
        tag = get_meqa_tag(desc)
        if tag != None:
            # a tag exist already, skip
            return

        # we try to find a class.property pair such that the normalized name of class and property appear
        # in the name of the property. If we can't do this, we try throw in the description field.
        # TODO, the description field can be big, we should do some syntactic analysis to it to trim it down.
        def match_to_name(all_words):
            for obj_name, obj in self.definitions.items():
                obj_words = set(obj_name.split(' '))
                if not obj_words < all_words:
                    continue
                for prop_norm_name, prop_orig_name in obj.properties.items():
                    prop_words = set(prop_norm_name.split(' '))
                    if not prop_words < all_words:
                        continue

                    # found one, update description and return. TODO try to find the best one
                    tag = MeqaTag(obj.name, prop_orig_name, '', 0)
                    param['description'] = desc + ' ' + tag.to_string()
                    return True
            return False

        norm_name = self.vocab.normalize(param.get('name'))
        norm_desc = self.vocab.normalize(desc)
        found = match_to_name(set(norm_name.split(' ')))
        if found:
            return

        found = match_to_name(set(norm_desc.split(' ')))
        if found:
            return

        match_to_name(set(norm_desc.split(' ')) | set(norm_name.split(' ')))

    def add_tags(self):
        # try to create tags and add them to the param's description field
        def create_tags_for_param(params):
            if params == None:
                return
            for p in params:
                self.guess_tag(path, method, p)

        paths = self.doc['paths']
        for pathname, path in paths.items():
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
            self.definitions[self.vocab.normalize(name)] = Definition(name, self)

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

    print("{} loaded", args.input)
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