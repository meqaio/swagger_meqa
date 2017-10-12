#!/bin/bash

pushd .

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

unameOut="$(uname -s)"
case "${unameOut}" in
    CYGWIN*)    sep=";";DIR=`cygpath -w $DIR`;;
    *)          sep=":"
esac

export GOPATH=${DIR}${sep}${GOPATH}

cd $DIR/src
go get -u github.com/kardianos/govendor
govendor sync
../build.sh

popd
