package main

import (
	"meqa/mqswag"
	"meqa/mqutil"
)

func main() {
	filePath := "d:\\src\\autoapi\\example-jsons\\petstore.json"
	mqutil.Logger = mqutil.NewStdLogger()

	swagger := mqswag.Swagger{}
	swagger.InitFromFile(filePath)
}
