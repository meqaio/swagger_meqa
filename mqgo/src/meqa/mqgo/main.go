package main

import (
	"flag"
	"fmt"
	"os"

	"meqa/mqplan"
	"meqa/mqswag"
	"meqa/mqutil"
	"path/filepath"
)

const (
	meqaDataDir     = "meqa_data"
	swaggerJSONFile = "swagger.yaml"
	testPlanFile    = "testplan.yaml"
	resultFile      = "result.yaml"
)

func main() {
	meqaPath := flag.String("d", meqaDataDir, "the directory that holds the meqa data and swagger.json files")
	swaggerFile := flag.String("s", swaggerJSONFile, "the swagger.json file name or URL")
	testPlanFile := flag.String("p", testPlanFile, "the test plan file name")
	resultFile := flag.String("r", resultFile, "the test result file name")
	testToRun := flag.String("t", "all", "the test to run")
	username := flag.String("u", "", "the username for basic HTTP authentication")
	password := flag.String("w", "", "the password for basic HTTP authentication")
	apitoken := flag.String("a", "", "the api token for bearer HTTP authentication")
	verbose := flag.Bool("v", false, "turn on verbose mode")

	flag.Parse()
	mqutil.Verbose = *verbose

	mqutil.Logger = mqutil.NewFileLogger(filepath.Join(*meqaPath, "mqgo.log"))
	mqutil.Logger.Println("starting mqgo")

	swaggerJsonPath := filepath.Join(*meqaPath, *swaggerFile)
	testPlanPath := filepath.Join(*meqaPath, *testPlanFile)
	resultPath := filepath.Join(*meqaPath, *resultFile)
	if _, err := os.Stat(swaggerJsonPath); os.IsNotExist(err) {
		fmt.Printf("can't load swagger file at the following location %s", swaggerJsonPath)
		return
	}
	if _, err := os.Stat(testPlanPath); os.IsNotExist(err) {
		fmt.Printf("can't load test plan file at the following location %s", testPlanPath)
		return
	}

	// Test loading swagger.json
	swagger, err := mqswag.CreateSwaggerFromURL(swaggerJsonPath, *meqaPath)
	if err != nil {
		mqutil.Logger.Printf("Error: %s", err.Error())
	}
	mqswag.ObjDB.Init(swagger)

	// Test loading test plan
	mqplan.Current.Username = *username
	mqplan.Current.Password = *password
	mqplan.Current.ApiToken = *apitoken
	err = mqplan.Current.InitFromFile(testPlanPath, &mqswag.ObjDB)
	if err != nil {
		mqutil.Logger.Printf("Error loading test plan: %s", err.Error())
	}

	if *testToRun == "all" {
		for _, testSuite := range mqplan.Current.SuiteList {
			mqutil.Logger.Printf("\n---\nTest suite: %s\n", testSuite.Name)
			fmt.Printf("\n---\nTest suite: %s\n", testSuite.Name)
			err := mqplan.Current.Run(testSuite.Name, nil)
			mqutil.Logger.Printf("err:\n%v", err)
		}
	} else {
		mqutil.Logger.Printf("\n---\nTest suite: %s\n", *testToRun)
		fmt.Printf("\n---\nTest suite: %s\n", *testToRun)
		err := mqplan.Current.Run(*testToRun, nil)
		mqutil.Logger.Printf("err:\n%v", err)
	}

	os.Remove(resultPath)
	mqplan.Current.WriteResultToFile(resultPath)
}
