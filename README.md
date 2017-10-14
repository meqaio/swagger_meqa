# OpenAPI Testing Meqanized

Meqa generates and runs test suites using your swagger/OpenAPI yaml spec. It makes REST API testing easy by generating useful test patterns - no coding needed.

## Demo

![gif](https://i.imgur.com/dT4qNMV.gif)

## Highlights

* Understands the object relationships and generates tests that use the right objects and values.
* Uses the description fields in the OpenAPI spec to understand the spec better and further improve accuracy. 
* Verifies the REST call results against known objects and values.
* Verifies the REST call results against OpenAPI schema.
* Produces easy to understand and easy to modify intermediate files for customization.

## Getting Started

The compiled binaries for Linux, Windows and MacOS are under this repo's "releases" directory. You can also docker pull meqa/go:latest.

There are two steps
* Use your swagger spec (e.g. swagger.yml) to generate the test suite files.
* Pick a test suite file to run.

Using downloaded binary the commands 
* mqgo generate -d testdata/ -s testdata/petstore.yml
* mqgo run -d testdata/ -s testdata/swagger_meqa.yml -p testdata/simple.yml

The run step uses swagger_meqa.yml, which is a tagged version of the original (petstore.yml in the above example).
* Search for meqa in swagger_meqa.yml to see all the tags.
* See docs for the meaning of tags and adjust them if a tag is wrong.
* The tags will be more accurate if the swagger is more structured (e.g. using #definitions instead of inline Objects) and has more descriptions.
* If you add or override the meqa tags, you can feed the tagged yaml file into the generate function again to create new test suites.

The run step takes a generated test suite file (simple.yml in the above example).
* simple.yml just exercises a few simple APIs to expose obvious issues, such as lack of api keys.
* path.yml exercises CRUD patterns grouped by the REST path.
* object.yml tries to create an object, then exercises the endpoints that needs the object as an input.
* The above are just the starting point as proof of concept. We will add more test suites (e.g. negative tests) if there are enough interest.
* The test yaml files can be edited to add in your own test suites. We allow overriding global, test suite and test parameters, as well as chaining output to input parameters. See doc for more details.

## Docs

For more details see the [docs](docs) directory.