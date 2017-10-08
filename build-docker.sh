#!/bin/bash

pushd .

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

cd $DIR

echo "building meqa docker images..."
#docker build -f docker/Dockerfile.server-base -t meqa/python:latest .
docker build -f docker/Dockerfile.server -t meqa/mqserver:latest .
docker build -f docker/Dockerfile.client -t meqa/go:latest .

popd
