#!/bin/bash

pushd .

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

unameOut="$(uname -s)"
case "${unameOut}" in
    CYGWIN*)    sep=";";DIR=`cygpath -w $DIR`;;
    *)          sep=":"
esac

export GOPATH=${GOPATH}${sep}${DIR}
echo "GOPATH=${GOPATH}"

function build_one {
    echo "building ${GOOS}_${GOARCH}"
    GOOS=${GOOS} GOARCH=${GOARCH} go install meqa/mqgo
    GOOS=${GOOS} GOARCH=${GOARCH} go install meqa/mqgen
}

export GOARCH=amd64
for GOOS in "windows" "linux" "darwin"
do
    build_one
done

popd
