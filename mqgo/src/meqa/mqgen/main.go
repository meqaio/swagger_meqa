package main

import (
	"flag"
	"meqa/mqplan"
	"os"

	"meqa/mqswag"
	"meqa/mqutil"
	"path/filepath"
)

const (
	meqaDataDir     = "meqa_data"
	swaggerJSONFile = "swagger.json"
	testPlanFile    = "testplan.yml"
)

func main() {
	mqutil.Logger = mqutil.NewStdLogger()

	meqaPath := flag.String("meqa", meqaDataDir, "the directory that holds the meqa data and swagger.json files")
	swaggerFile := flag.String("swagger", swaggerJSONFile, "the swagger.json file name or URL")
	testPlanFile := flag.String("testplan", testPlanFile, "the test plan file name")

	flag.Parse()
	swaggerJsonPath := filepath.Join(*meqaPath, *swaggerFile)
	testPlanPath := filepath.Join(*meqaPath, *testPlanFile)
	if _, err := os.Stat(swaggerJsonPath); os.IsNotExist(err) {
		mqutil.Logger.Printf("can't load swagger file at the following location %s", swaggerJsonPath)
		return
	}
	if _, err := os.Stat(testPlanPath); !os.IsNotExist(err) {
		mqutil.Logger.Printf("test plan file exists at the following location %s", testPlanPath)
		return
	}

	// loading swagger.json
	swagger, err := mqswag.CreateSwaggerFromURL(swaggerJsonPath)
	if err != nil {
		mqutil.Logger.Printf("Error: %s", err.Error())
	}

	mqplan.Current.Init(swagger, nil)
}
