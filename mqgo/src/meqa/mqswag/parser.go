// This package handles swagger.json file parsing
package mqswag

import (
	"encoding/json"
	"io/ioutil"
	"meqa/mqutil"
)

// string fields in OpenAPI doc
const SWAGGER = "swagger"
const SCHEMES = "schemes"
const CONSUMES = "consumes"
const PRODUCES = "produces"
const INFO = "info"
const HOST = "host"
const BASEPATH = "basePath"
const TAGS = "tags"
const PATHS = "paths"
const SECURITY_DEFINITIONS = "securityDefinitions"
const DEFINITIONS = "definitions"

// Swagger stores the swagger.json we parsed.
type Swagger struct {
	// The raw doc parsed from json.
	doc map[string]interface{}
}

// Init from a json string
func (swagger *Swagger) InitFromString(data []byte) error {
	swagger.doc = nil
	err := json.Unmarshal(data, &swagger.doc)
	if err != nil {
		mqutil.Logger.Println("The input is not a valid json")
		mqutil.Logger.Println(err.Error())
		return err
	}
	return nil
}

// Init from a file
func (swagger *Swagger) InitFromFile(path string) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		mqutil.Logger.Printf("Can't open the following file: %s", path)
		mqutil.Logger.Println(err.Error())
		return err
	}
	return swagger.InitFromString(data)
}
