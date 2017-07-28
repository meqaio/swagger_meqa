package main

import (
	"encoding/json"
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

	mqswag.ObjDB.Init(swagger)

	// Test loading test plan
	err = mqplan.Current.InitFromFile(testPlanPath, &mqswag.ObjDB)
	if err != nil {
		mqutil.Logger.Printf("Error loading test plan: %s", err.Error())
	}

	fmt.Println("\n====== running get pet by status ======")
	result, err := mqplan.Current.Run("get pet by status", nil)
	resultJson, _ := json.Marshal(result)
	fmt.Printf("\nresult:\n%s", resultJson)

	fmt.Println("\n====== running create user manual ======")
	result, err = mqplan.Current.Run("create user manual", nil)
	resultJson, _ = json.Marshal(result)
	fmt.Printf("\nresult:\n%s", resultJson)

	fmt.Printf("\nerr:\n%v", err)
}
