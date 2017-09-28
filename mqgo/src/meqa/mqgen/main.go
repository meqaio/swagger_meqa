package main

import (
	"flag"
	"fmt"
	"meqa/mqplan"
	"os"

	"meqa/mqswag"
	"meqa/mqutil"
	"path/filepath"
)

const (
	meqaDataDir     = "meqa_data"
	swaggerJSONFile = "swagger.yaml"
	testPlanFile    = "testplan.yaml"
)

func main() {
	mqutil.Logger = mqutil.NewStdLogger()

	meqaPath := flag.String("d", meqaDataDir, "the directory that holds the meqa data and swagger.json files")
	swaggerFile := flag.String("s", swaggerJSONFile, "the swagger.json file name")
	testPlanFile := flag.String("p", testPlanFile, "the test plan file name")
	algorithm := flag.String("a", "path", "the algorithm - object, path")
	verbose := flag.Bool("v", false, "turn on verbose mode")

	flag.Parse()
	mqutil.Verbose = *verbose
	swaggerJsonPath := filepath.Join(*meqaPath, *swaggerFile)
	testPlanPath := filepath.Join(*meqaPath, *testPlanFile)
	if _, err := os.Stat(swaggerJsonPath); os.IsNotExist(err) {
		fmt.Printf("Can't load swagger file at the following location %s", swaggerJsonPath)
		return
	}
	if _, err := os.Stat(testPlanPath); !os.IsNotExist(err) {
		fmt.Printf("Test plan file exists: %s. Please remove old test plan files and try again.", testPlanPath)
		return
	}

	// loading swagger.json
	swagger, err := mqswag.CreateSwaggerFromURL(swaggerJsonPath, *meqaPath)
	if err != nil {
		mqutil.Logger.Printf("Error: %s", err.Error())
		return
	}
	dag := mqswag.NewDAG()
	err = swagger.AddToDAG(dag)
	if err != nil {
		mqutil.Logger.Printf("Error: %s", err.Error())
		return
	}

	dag.Sort()
	dag.CheckWeight()

	var testPlan *mqplan.TestPlan
	if *algorithm == "path" {
		testPlan, err = mqplan.GeneratePathTestPlan(swagger, dag)
	} else {
		testPlan, err = mqplan.GenerateTestPlan(swagger, dag)
	}
	if err != nil {
		mqutil.Logger.Printf("Error: %s", err.Error())
		return
	}

	err = testPlan.DumpToFile(testPlanPath)
	if err != nil {
		mqutil.Logger.Printf("Error: %s", err.Error())
		return
	}

	fmt.Println("Test plans generated in directory:", *meqaPath)
}
