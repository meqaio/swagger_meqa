package main

import (
	"fmt"
	"meqa/mqswag"
	"meqa/mqutil"
)

func main() {
	filePath := "d:\\src\\autoapi\\example-jsons\\petstore.json"
	mqutil.Logger = mqutil.NewStdLogger()

	swagger, err := mqswag.CreateSwaggerFromURL(filePath)
	if err != nil {
		mqutil.Logger.Printf("Error: %s", err.Error())
	}
	for pathName, pathItem := range swagger.Paths.Paths {
		fmt.Printf("%v:%v\n", pathName, pathItem)
	}
	fmt.Printf("%v", swagger.Paths.Paths["/pet"].Post)
}
