# Installing and Running Meqa

Meqa takes an OpenAPI (formerly Swagger) spec, parses it to understand the structure, the relationship among objects and operations, and generate test suites. It is in its early stage, and only works with OpenAPI version 2.0.

Meqa achieves its goal in three steps
* Add <meqa ... > tags to the OpenAPI spec in YAML format to indicate meqa's understanding of the structure of the spec.
* Use the above tagged OpenAPI spec to generate test suites in yaml.
* Run test suites.

The easiest way to try meqa is to install mqgo, which is a small binary. It sends your OpenAPI spec to the demo server (https://api.meqa.io) to be processed and generate the test suites. Mqgo will run your tests locally, so your test target can still be on a private IP or inside a firewall.

## Installing mqgo

The compiled mqgo binaries are under the "releases" directory of this repo.

If you want to use the Docker containers. The equivalent of the "mqgo" binary is the meqa/go container.
* docker pull meqa/go:latest

## Running mqgo and Using https://api.meqa.io

In this example we use the standard [petstore demo](http://petstore.swagger.io). The OpenAPI spec is saved at /testdata/petstore.yml. The first command below sends petstore.yml to api.meqa.io to generate petstore_meqa.yml and test plan yaml files (e.g. path.yml) under the /testdata directory. The second command runs the test suites in path.yml test plan.

* mqgo generate -d /testdata -s /testdata/petstore.yml
* mqgo run -d /testdata -s /testdata/petstore_meqa.yml -p /testdata/path.yml

To use Docker container the commands would be
* docker run -it -v /testdata:/testdata meqa/go mqgo generate -d /testdata -s /testdata/petstore.yml
* docker run -it -v /testdata:/testdata meqa/go mqgo run -d /testdata -s /testdata/petstore_meqa.yml -p /testdata/path.yml

The meqa tag and test plan file format are explained in the [Meqa Format](format.md) doc.

## Running Everything Locally

To run everything on your local computer, you need mqtag, mqgen and mqgo.

### Build/Install Locally

* mqgen and mqgo
    * You need to have golang 1.8+ installed. Run mqgo/build-vendor.sh.
    * To use Docker container instead of the above, docker pull meqa/go:latest.
* mqtag - Note that building mqtag would take a while, because the NLP libraries are big.
    * You need to have python 3.5+ installed. Run: pip3 install mqtag
    * python3 -m spacy download en_core_web_md
    * Run the following and replace <python_lib_installdir> with your actuall install directory (e.g. /usr/local/lib/python3.6/site-packages) : echo -100000000 > <python_lib_install_dir>/en_core_web_md/en_core_web_md-1.2.1/vocab/oov_prob
    * To use Docker container instead of the above, docker pull meqa/tag:latest

### Run Locally

In this example we use the standard petstore demo. The OpenAPI spec is /testdata/petstore.yml. The first command below takes petstore.yml as input and generates petstore_meqa.yml as output. The second command uses petstore_meqa.yml to generate the test plan yaml files (e.g. path.yml)under the /testdata directory. The third command runs the test suites in path.yml test plan.

* mqtag -i /testdata/petstore.yml -o /testdata/petstore_meqa.yml
* mqgen -d /testdata -s /testdata/petstore_meqa.yml
* mqgo run -d /testdata -s /testdata/petstore_meqa.yml -p /testdata/path.yml

To use Docker container the commands would be
* docker run -it -v /testdata:/testdata meqa/tag mqtag -i /testdata/petstore.yml -o /testdata/petstore_meqa.yml
* docker run -it -v /testdata:/testdata meqa/go mqgen -d /testdata -s /testdata/petstore_meqa.yml
* docker run -it -v /testdata:/testdata meqa/go mqgo run -d /testdata -s /testdata/petstore_meqa.yml -p /testdata/path.yml

The meqa tag and test file format are explained in the [meqa Format](format.md) doc.
