#!/bin/bash

pushd .

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

export GOPATH=$GOPATH:$DIR

cd $DIR/src
go get -u github.com/kardianos/govendor
govendor sync
../build.sh

popd
