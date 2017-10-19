package main

import (
	"meqa/mqutil"
	"os"
	"path/filepath"
	"testing"
)

func TestMqgo(t *testing.T) {
	wd, _ := os.Getwd()
	meqaPath := filepath.Join(wd, "../../../testdata")
	swaggerPath := filepath.Join(meqaPath, "petstore_meqa.yml")
	planPath := filepath.Join(meqaPath, "object.yml")
	resultPath := filepath.Join(meqaPath, "result.yml")
	testToRun := "all"
	username := ""
	password := ""
	apitoken := ""
	verbose := false

	mqutil.Logger = mqutil.NewFileLogger(filepath.Join(meqaPath, "mqgo.log"))
	runMeqa(&meqaPath, &swaggerPath, &planPath, &resultPath, &testToRun, &username, &password, &apitoken, &verbose)
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
