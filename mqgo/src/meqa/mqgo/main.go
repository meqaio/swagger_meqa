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
)

func main() {
	mqutil.Logger = mqutil.NewStdLogger()

	meqaPath := flag.String("meqa", meqaDataDir, "the directory that holds the meqa data and swagger.json files")
	swaggerFile := flag.String("swagger", swaggerJSONFile, "the swagger.json file name or URL")
	testPlanFile := flag.String("testplan", testPlanFile, "the test plan file name")
	testToRun := flag.String("test", "all", "the test to run")
	username := flag.String("username", "", "the username for basic HTTP authentication")
	password := flag.String("password", "", "the password for basic HTTP authentication")

	flag.Parse()
	swaggerJsonPath := filepath.Join(*meqaPath, *swaggerFile)
	testPlanPath := filepath.Join(*meqaPath, *testPlanFile)
	if _, err := os.Stat(swaggerJsonPath); os.IsNotExist(err) {
		mqutil.Logger.Printf("can't load swagger file at the following location %s", swaggerJsonPath)
		return
	}
	if _, err := os.Stat(testPlanPath); os.IsNotExist(err) {
		mqutil.Logger.Printf("can't load test plan file at the following location %s", testPlanPath)
		return
	}

	// Test loading swagger.json
	swagger, err := mqswag.CreateSwaggerFromURL(swaggerJsonPath, *meqaPath)
	if err != nil {
		mqutil.Logger.Printf("Error: %s", err.Error())
	}
	for pathName, pathItem := range swagger.Paths.Paths {
		fmt.Printf("%v:%v\n", pathName, pathItem)
	}
	fmt.Printf("%v", swagger.Paths.Paths["/pet"].Post)

	mqswag.ObjDB.Init(swagger)

	// Test loading test plan
	err = mqplan.Current.InitFromFile(testPlanPath, &mqswag.ObjDB)
	if err != nil {
		mqutil.Logger.Printf("Error loading test plan: %s", err.Error())
	}
	mqplan.Current.Username = *username
	mqplan.Current.Password = *password

	if *testToRun == "all" {
		for _, testCase := range mqplan.Current.CaseList {
			mqutil.Logger.Printf("\n\n======================== Running test case: %s ========================\n", testCase.Name)
			err := mqplan.Current.Run(testCase.Name, nil)
			mqutil.Logger.Printf("err:\n%v", err)
		}
	} else {
		err := mqplan.Current.Run(*testToRun, nil)
		mqutil.Logger.Printf("err:\n%v", err)
	}
}
