#!/usr/bin/bash
VERSION="0.8"
docker build -f Dockerfile.server-base -t meqa/python:$VERSION .
docker build -f Dockerfile.server -t yingxie3/mqserver:$VERSION .