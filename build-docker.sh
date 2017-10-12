#!/bin/bash

pushd .

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

cd $DIR

echo "building meqa docker images..."
#docker build -f mqserver/Dockerfile.base -t meqa/python:latest .
docker build -f mqserver/Dockerfile -t meqa/mqserver:latest .
docker build -f mqgo/Dockerfile -t meqa/go:latest .

popd
