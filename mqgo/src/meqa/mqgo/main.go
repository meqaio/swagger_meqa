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
	meqaDataDir = "meqa_data"
	swaggerYAML = "swagger.yaml"
	resultFile  = "result.yaml"
)

func main() {
	swaggerYAMLPath := filepath.Join(meqaDataDir, swaggerYAML)
	testPlanPath := filepath.Join(meqaDataDir, "simple.yaml")
	resultPath := filepath.Join(meqaDataDir, resultFile)

	meqaPath := flag.String("d", meqaDataDir, "the directory where we put meqa temp files and logs")
	swaggerFile := flag.String("s", swaggerYAMLPath, "the swagger.yaml file name or URL")
	testPlanFile := flag.String("p", testPlanPath, "the test plan file name")
	resultFile := flag.String("r", resultPath, "the test result file name")
	testToRun := flag.String("t", "all", "the test to run")
	username := flag.String("u", "", "the username for basic HTTP authentication")
	password := flag.String("w", "", "the password for basic HTTP authentication")
	apitoken := flag.String("a", "", "the api token for bearer HTTP authentication")
	verbose := flag.Bool("v", false, "turn on verbose mode")

	flag.Parse()
	mqutil.Verbose = *verbose

	mqutil.Logger = mqutil.NewFileLogger(filepath.Join(*meqaPath, "mqgo.log"))
	mqutil.Logger.Println("starting mqgo")

	if _, err := os.Stat(*swaggerFile); os.IsNotExist(err) {
		fmt.Printf("can't load swagger file at the following location %s", *swaggerFile)
		return
	}
	if _, err := os.Stat(*testPlanFile); os.IsNotExist(err) {
		fmt.Printf("can't load test plan file at the following location %s", *testPlanFile)
		return
	}

	// Test loading swagger.json
	swagger, err := mqswag.CreateSwaggerFromURL(*swaggerFile, *meqaPath)
	if err != nil {
		mqutil.Logger.Printf("Error: %s", err.Error())
	}
	mqswag.ObjDB.Init(swagger)

	// Test loading test plan
	mqplan.Current.Username = *username
	mqplan.Current.Password = *password
	mqplan.Current.ApiToken = *apitoken
	err = mqplan.Current.InitFromFile(*testPlanFile, &mqswag.ObjDB)
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

	os.Remove(*resultFile)
	mqplan.Current.WriteResultToFile(*resultFile)
}
