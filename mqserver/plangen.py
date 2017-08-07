import bravado_core
import pdb
import argparse
import json

from bravado_core.spec import Spec

# An object definition. Each Def has a weight. The weight indicates what other Defs it depends on.
#class Def:

def main():
    parser = argparse.ArgumentParser(description='generate testplan.yml from swagger.json')
    parser.add_argument("-i", "--input", help="the swagger.json file", default="./meqa_data/swagger.json")
    parser.add_argument("-o", "--output", help="the output file name", default="./meqa_data/testplan.yml")
    args = parser.parse_args()

    spec_dict = json.loads(open(args.input, 'r').read())
    spec = Spec.from_dict(spec_dict)

    for resourceName, resource in spec.resources.items():
        for operationId in resource.operations:
            operation = resource.operations.get(operationId)
            print("{}: {}".format(resourceName, operationId))

    for k, v in spec.definitions.items():
        print("{}: {}".format(k, v))

if __name__ == '__main__':
    #pdb.set_trace()
    main()