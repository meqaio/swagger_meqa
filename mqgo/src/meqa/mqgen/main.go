package main

import (
	"flag"
	"fmt"
	"meqa/mqplan"
	"os"
	"path/filepath"

	"meqa/mqswag"
	"meqa/mqutil"
)

const (
	meqaDataDir = "meqa_data"
	algoSimple  = "simple"
	algoObject  = "object"
	algoPath    = "path"
	algoAll     = "all"
)

var algoList []string = []string{algoSimple, algoObject, algoPath}

func main() {
	mqutil.Logger = mqutil.NewStdLogger()

	swaggerJSONFile := filepath.Join(meqaDataDir, "swagger.yaml")
	meqaPath := flag.String("d", meqaDataDir, "the directory where we put the generated files")
	swaggerFile := flag.String("s", swaggerJSONFile, "the swagger.yaml file location")
	algorithm := flag.String("a", "path", "the algorithm - simple, object, path, all")
	verbose := flag.Bool("v", false, "turn on verbose mode")

	flag.Parse()
	mqutil.Verbose = *verbose

	swaggerJsonPath := *swaggerFile
	if fi, err := os.Stat(swaggerJsonPath); os.IsNotExist(err) || fi.Mode().IsDir() {
		fmt.Printf("Can't load swagger file at the following location %s", swaggerJsonPath)
		os.Exit(1)
	}
	testPlanPath := *meqaPath
	if fi, err := os.Stat(testPlanPath); os.IsNotExist(err) {
		err = os.Mkdir(testPlanPath, 0755)
		if err != nil {
			fmt.Printf("Can't create the directory at %s\n", testPlanPath)
			os.Exit(1)
		}
	} else if !fi.Mode().IsDir() {
		fmt.Printf("The specified location is not a directory: %s\n", testPlanPath)
		os.Exit(1)
	}

	// loading swagger.json
	swagger, err := mqswag.CreateSwaggerFromURL(swaggerJsonPath, *meqaPath)
	if err != nil {
		mqutil.Logger.Printf("Error: %s", err.Error())
		os.Exit(1)
	}
	dag := mqswag.NewDAG()
	err = swagger.AddToDAG(dag)
	if err != nil {
		mqutil.Logger.Printf("Error: %s", err.Error())
		os.Exit(1)
	}

	dag.Sort()
	dag.CheckWeight()

	var plansToGenerate []string
	if *algorithm == algoAll {
		plansToGenerate = algoList
	} else {
		plansToGenerate = append(plansToGenerate, *algorithm)
	}

	for _, algo := range plansToGenerate {
		var testPlan *mqplan.TestPlan
		switch algo {
		case algoPath:
			testPlan, err = mqplan.GeneratePathTestPlan(swagger, dag)
		case algoObject:
			testPlan, err = mqplan.GenerateTestPlan(swagger, dag)
		default:
			testPlan, err = mqplan.GenerateSimpleTestPlan(swagger, dag)
		}
		if err != nil {
			mqutil.Logger.Printf("Error: %s", err.Error())
			os.Exit(1)
		}
		testPlanFile := filepath.Join(testPlanPath, algo+".yaml")
		err = testPlan.DumpToFile(testPlanFile)
		if err != nil {
			mqutil.Logger.Printf("Error: %s", err.Error())
			os.Exit(1)
		}
		fmt.Println("Test plans generated at:", testPlanFile)
	}
}
