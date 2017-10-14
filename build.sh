#!/bin/bash

pushd .

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

cd $DIR

echo "building mqgo directory..."
mqgo/build-vendor.sh

echo "building meqa docker images..."
docker build -f mqtag/Dockerfile -t meqa/tag:latest .
docker build -f mqgo/Dockerfile -t meqa/go:latest .

popd
