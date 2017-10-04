#!/usr/bin/bash

go install meqa/mqgo
go install meqa/mqgen
go install meqa/mqtag

export GOOS=linux
export GOARCH=amd64
go install meqa/mqgo
go install meqa/mqgen
go install meqa/mqtag

