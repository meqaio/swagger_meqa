# Installing and Running Meqa

Meqa takes a Swagger/OpenAPI spec, parses it to understand the structure, the relationship among objects and operations, and generate test suites. It is in its early stage, and only works with swagger version 2.0.

Meqa achieves its goal in three steps
* Add <meqa ... > tags to the Swagger yaml spec to indicate meqa's understanding of the structure of the spec.
* Use the above tagged Swagger spec to generate test suites in yaml.
* Run a test suite.

The easiest way to try meqa is to install mqgo, which is a small binary. It sends your Swagger spec to the demo server (https://api.meqa.io) to be processed and generate the test suites. Mqgo will run your tests locally, so your test target can still be on a private IP or inside a firewall.

## Installing mqgo

The compiled mqgo binaries are under the "releases" directory of this repo.

If you want to use the Docker containers. The equivalent of the "mqgo" binary is the meqa/go container.
* docker pull meqa/go:latest

## Running mqgo and Using https://api.meqa.io

In this example we use the Swagger's demo petstore. The Swagger spec is at /testdata/petstore.yml. The first command below sends petstore.yml to api.meqa.io to generate swagger_meqa.yml and test suite yaml files (e.g. simple.yml) under the /testdata directory. The second command runs the tests in simple.yml test suite.

* mqgo generate -d /testdata -s /testdata/petstore.yml
* mqgo run -d /testdata -s /testdata/swagger_meqa.yml -p /testdata/simple.yml

To use Docker container the commands would be
* docker run -it -v /testdata:/testdata meqa/go mqgo generate -d /testdata -s /testdata/petstore.yml
* docker run -it -v /testdata:/testdata meqa/go mqgo run -d /testdata -s /testdata/swagger_meqa.yml -p /testdata/simple.yml

The meqa tag and test suite format are explained in the [Meqa Format](format.md) doc.

## Running Everything Locally

To run everything on your local computer, you need mqtag, mqgen and mqgo.

### Build/Install Locally

* mqgen and mqgo
    * Run mqgo/build-vendor.sh.
    * To use Docker container instead of the above, docker pull meqa/go:latest.
* mqtag - Note that building mqtag would take a while, because the NLP libraries are big.
    * pip3 install mqtag
    * python3 -m spacy download en_core_web_md
    * Run the following and replace <python_lib_installdir> with your actuall install directory (e.g. /usr/local/lib/python3.6/site-packages) : echo -100000000 > <python_lib_install_dir>/en_core_web_md/en_core_web_md-1.2.1/vocab/oov_prob
    * To use Docker container instead of the above, docker pull meqa/tag:latest

### Run Locally

In this example we use the Swagger's demo petstore. The Swagger spec is /testdata/petstore.yml. The first command below takes petstore.yml as input and generates swagger_meqa.yml as output. The second command uses swagger_meqa.yml to generate the test suite yaml files (e.g. simple.yml)under the /testdata directory. The third command runs the tests in simple.yml test suite.

* mqtag -i /testdata/petstore.yml -o /testdata/swagger_meqa.yml
* mqgen -d /testdata -s /testdata/swagger_meqa.yml
* mqgo run -d /testdata -s /testdata/swagger_meqa.yml -p /testdata/simple.yml

To use Docker container the commands would be
* docker run -it -v /testdata:/testdata meqa/tag mqtag -i /testdata/petstore.yml -o /testdata/swagger_meqa.yml
* docker run -it -v /testdata:/testdata meqa/go mqgen -d /testdata -s /testdata/swagger_meqa.yml
* docker run -it -v /testdata:/testdata meqa/go mqgo run -d /testdata -s /testdata/swagger_meqa.yml -p /testdata/simple.yml

The meqa tag and test suite format are explained in the [Meqa Format](format.md) doc.
