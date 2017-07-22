package main

import (
	"fmt"

	"meqa/mqplan"
	"meqa/mqswag"
	"meqa/mqutil"
)

func main() {
	swaggerJsonPath := "d:\\src\\autoapi\\example-jsons\\petstore.json"
	testPlanPath := "d:\\src\\autoapi\\docs\\test-plan-example.yml"

	mqutil.Logger = mqutil.NewStdLogger()

	// Test loading swagger.json
	swagger, err := mqswag.CreateSwaggerFromURL(swaggerJsonPath)
	if err != nil {
		mqutil.Logger.Printf("Error: %s", err.Error())
	}
	for pathName, pathItem := range swagger.Paths.Paths {
		fmt.Printf("%v:%v\n", pathName, pathItem)
	}
	fmt.Printf("%v", swagger.Paths.Paths["/pet"].Post)

	// Test loading test plan
	err = mqplan.Current.InitFromFile(testPlanPath)
	if err != nil {
		mqutil.Logger.Printf("Error loading test plan: %s", err.Error())
	}

	mqplan.Current.Run("create user manual", swagger, mqswag.DB)
}
