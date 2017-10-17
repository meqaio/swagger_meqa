#!/bin/bash

pushd .

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

unameOut="$(uname -s)"
case "${unameOut}" in
    CYGWIN*)    sep=";";DIR=`cygpath -w $DIR`;;
    *)          sep=":"
esac

if [ -z ${GOPATH} ]; then
    export GOPATH=${DIR}
else
    export GOPATH=${DIR}${sep}${GOPATH}
fi
export PATH=${PATH}${sep}${DIR}/bin
echo "GOPATH=${GOPATH}"

cd $DIR
go test meqa/mqgen
go test meqa/mqgo

popd

