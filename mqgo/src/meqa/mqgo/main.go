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

	// Test loading test plan
	err = mqplan.Current.InitFromFile(testPlanPath)
	if err != nil {
		mqutil.Logger.Printf("Error loading test plan: %s", err.Error())
	}

	param, err := mqplan.GenerateParameter(&(swagger.Paths.Paths["/pet/findByStatus"].Get.Parameters[0]), swagger, mqswag.ObjDB)
	str, _ := json.MarshalIndent(param, "", "    ")
	if err == nil {
		fmt.Printf("\n---\n%s", str)
	} else {
		fmt.Printf("\nerr:\n%v", err)
	}

	schema, err := mqplan.GenerateSchema("", swagger.Paths.Paths["/pet"].Post.Parameters[0].Schema, swagger, mqswag.ObjDB)
	str, _ = json.MarshalIndent(schema, "", "    ")
	if err == nil {
		fmt.Printf("\n---\n%s", str)
	} else {
		fmt.Printf("\nerr:\n%v", err)
	}
	fmt.Println("====== running get pet by status ======")
	result, err := mqplan.Current.Run("get pet by status", swagger, mqswag.ObjDB, nil)
	resultJson, _ := json.Marshal(result)
	fmt.Printf("\nresult:\n%s", resultJson)

	fmt.Printf("\nerr:\n%v", err)
}
