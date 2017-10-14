#!/bin/bash

pushd .

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

cd $DIR

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
echo "GOPATH=${GOPATH}"

function build_one {
    if [ ${GOOS} = "windows" ]; then
        EXT=".exe"
    else
        EXT=""
    fi

    echo "building ${GOOS}_${GOARCH}"
    GOOS=${GOOS} GOARCH=${GOARCH} go build -o bin/${GOOS}_${GOARCH}/mqgo${EXT} meqa/mqgo
    GOOS=${GOOS} GOARCH=${GOARCH} go build -o bin/${GOOS}_${GOARCH}/mqgen${EXT} meqa/mqgen
}

export GOARCH=amd64
for GOOS in "windows" "linux" "darwin"
do
    build_one
done

popd
