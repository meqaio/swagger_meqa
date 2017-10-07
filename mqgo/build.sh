#!/bin/bash

pushd .

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

export GOPATH=$GOPATH:$DIR

export GOOS=linux
export GOARCH=amd64
go install meqa/mqgo
go install meqa/mqgen

export GOOS=windows
export GOARCH=amd64
go install meqa/mqgo
go install meqa/mqgen

export GOOS=darwin
export GOARCH=amd64
go install meqa/mqgo
go install meqa/mqgen

popd
