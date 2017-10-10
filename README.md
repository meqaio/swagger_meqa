# OpenAPI Testing Meqanized

Meqa generates and runs test suites using your swagger/OpenAPI yaml spec. It makes REST API testing easy by generating useful test patterns - no coding needed.

## Highlights

* Understands the object relationships and generates tests that use the right objects and values.
* Uses the description fields in the OpenAPI spec to understand the spec better and further improve accuracy. 
* Verifies the REST call results against known objects and values.
* Verifies the REST call results against OpenAPI schema.
* Produces easy to understand and easy to modify intermediate files for customization.

## Getting Started

The compiled binaries for Linux, Windows and MacOS are under releases directory. You can also docker pull meqa/go:latest.

There are two steps
* Use your swagger.yaml file to generate the test suite files.
* Pick a test suite file to run.

Using downloaded binary the commands would be
* bin/mqgo.exe generate -d /tmp/meqa_data -s ../example-jsons/petstore.yaml
* bin/mqgo.exe run -d /tmp/meqa_data/ -s /tmp/meqa_data/swagger_meqa.yaml -p /tmp/meqa_data/simple.yaml

The run step uses swagger_meqa.yaml, which is a tagged version of the original.
* Search for meqa in the file to see all the tags.
* See docs for the meaning of tags and adjust them if a tag is wrong.
* The tags will be more accurate if the swagger is more structured (using #definitions instead of inline Objects), has more descriptions, and as we employ more NLP technics.
* If you add or override the meqa tags, you can feed the tagged yaml into generate function again to create new test suites.

The run step takes a generated test suite file (simple.yaml in the above example).
* simple.yaml just exercises a few simple APIs to expose obvious issues, such as lack of api keys.
* path.yaml exercises CRUD patterns grouped by the REST path.
* object.yaml tries to create an object, then exercises the endpoints that needs the object as an input.
* The above are just the starting point as proof of concept. We will add more test suites (e.g. negative tests) if there are enough interest.
* The test yaml files can be edited to add in your own test suites. We allow overriding global, test suite and test parameters, as well as chaining output to input parameters.

