# Building the Project

Swagger_meqa is written mostly in golang, with the meqa tag generation part in python because most of the NLP libraries are in python.

## Building Golang

* Need golang 1.8+
* Run mqgo/build-vendor.sh - the command would download govendor into your current GOPATH, and run govendor to download the project dependencies. It would take some time depending on your network. Your current GOPATH/bin should be in your PATH.
* The binaries would be under mqgo/bin

## Running python

* Need python 3.5+
* Downloading python dependencies 