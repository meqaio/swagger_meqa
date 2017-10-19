# OpenAPI Testing Meqanized

Meqa generates and runs test suites using your OpenAPI (formerly Swagger) spec in YAML. It makes REST API testing easy by generating useful test patterns - no coding needed.

## Demo

![gif](https://i.imgur.com/prWsMEi.gif)

## Highlights

* Understands the object relationships and generates tests that use the right objects and values.
* Uses the description fields in the OpenAPI spec to understand the spec better and further improve accuracy. 
* Verifies the REST call results against known objects and values.
* Verifies the REST call results against OpenAPI schema.
* Produces easy to understand and easy to modify intermediate files for customization.

## Getting Started

The compiled binaries for Linux, Windows and MacOS are under [releases](https://github.com/meqaio/swagger_meqa/releases). You can also docker pull meqa/go:latest. In the examples below we use the classic [petstore example spec] (http://petstore.swagger.io/).

There are two steps.
* Use your OpenAPI spec (e.g., petstore.yml) to generate the test plan files.
* Pick a test plan file to run.

The commands are:
* mqgo generate -d /testdata/ -s /testdata/petstore.yml
* mqgo run -d /testdata/ -s /testdata/petstore_meqa.yml -p /testdata/path.yml

The run step uses petstore_meqa.yml, which is a tagged version of the original petstore.yml.
* Search for meqa in petstore_meqa.yml to see all the tags.
* The tags will be more accurate if the OpenAPI spec is more structured (e.g. using #definitions instead of inline Objects) and has more descriptions.
* See [meqa Format](docs/format.md) for the meaning of tags and adjust them if a tag is wrong.
* If you add or override the meqa tags, you can feed the tagged yaml file into the "mqgo generate" function again to create new test suites.

The run step takes a generated test plan file (path.yml in the above example).
* simple.yml just exercises a few simple APIs to expose obvious issues, such as lack of api keys.
* path.yml exercises CRUD patterns grouped by the REST path.
* object.yml tries to create an object, then exercises the endpoints that needs the object as an input.
* The above are just the starting point as proof of concept. We will add more test patterns if there are enough interest.
* The test yaml files can be edited to add in your own test suites. We allow overriding global, test suite and test parameters, as well as chaining output to input parameters. See [meqa format](docs/format.md) for more details.

## Docs

For more details see the [docs](docs) directory.

