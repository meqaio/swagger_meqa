import pdb
import argparse
import json
import logging
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

class Definition(object):
    def __init__(self, name, swagger):
        self.name = name
        orig_properties = swagger.get_properties(swagger.doc['definitions'][name])
        self.properties = dict()

        for orig_prop in orig_properties:
            norm_prop = swagger.vocab.normalize_name(orig_prop)
            self.properties[norm_prop] = orig_prop

class SwaggerDoc(object):
    def __init__(self, filename):
        self.vocab = Vocabulary()
        self.doc = load_yaml(filename)
        self.definitions = dict()  # holds the object definitions.
        self.logger = logging.getLogger(name='meqa')

    # get the properties (set) from the schema
    def get_properties(self, schema):
        if 'allOf' in schema:
            properties = set()
            for s in schema['allOf']:
                properties = properties.union(self.get_properties(s))
            return properties

        if '$ref' in schema:
            refstr = schema['$ref']
            # we only handle local refs for now
            if refstr[0] != '#':
                return set()

            reflist = refstr.split('/')
            refschema = self.doc[reflist[1]][reflist[2]]
            return self.get_properties(refschema)

        properties = set()
        if 'properties' in schema:
            for pname in schema['properties']:
                properties.add(pname)
        return properties


    def gather_words(self):
        # we have to do two passes. First time we add all the words into the vocabulary. The second
        # pass we may use the new vocabulary to break down some words that may not be breakable before.
        for name, obj in self.doc['definitions'].items():
            self.vocab.add_word(name)
            if 'properties' in obj:
                for key in obj['properties']:
                    self.vocab.add_word(key)

        # second pass, we lemmarize the unit words and use them as key
        for name in self.doc['definitions']:
            self.definitions[self.vocab.normalize_name(name)] = Definition(name, self)

def main():
    logger = logging.getLogger(name='meqa')
    parser = argparse.ArgumentParser(description='generate tag.yaml from swagger.yaml')
    parser.add_argument("-i", "--input", help="the swagger.yaml file", default="./meqa_data/swagger.yaml")
    parser.add_argument("-o", "--output", help="the output file name", default="./meqa_data/tag.yaml")
    args = parser.parse_args()

    swagger = SwaggerDoc(args.input)
    if swagger.doc == None:
        logger.error("Failed to load file %s", args.input)
        return

    swagger.gather_words()

    for norm_name, obj in swagger.definitions.items():
        print("\n======== definition {} -> {} =========\n".format(norm_name, obj.name))
        print()

        for prop_norm_name, prop_orig_name in obj.properties.items():
            print("{} -> {}\n".format(prop_norm_name, prop_orig_name))
    print("done")

if __name__ == '__main__':
    vocab = Vocabulary()
    vocab.add_word('uuid')
    print(vocab.normalize_name('uuid'))
    main()