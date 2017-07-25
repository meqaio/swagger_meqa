// This package handles swagger.json file parsing
package mqswag

import (
	"log"
	"meqa/mqutil"

	"github.com/go-openapi/loads"
	"github.com/go-openapi/loads/fmts"
	"github.com/go-openapi/spec"
)

// string fields in OpenAPI doc
/*
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
*/

type Swagger spec.Swagger

// Init from a file
func CreateSwaggerFromURL(path string) (*Swagger, error) {
	specDoc, err := loads.Spec(path)
	if err != nil {
		mqutil.Logger.Printf("Can't open the following file: %s", path)
		mqutil.Logger.Println(err.Error())
		return nil, err
	}

	log.Println("Would be serving:", specDoc.Spec().Info.Title)

	return (*Swagger)(specDoc.Spec()), nil
}

// AddSchemasToDB finds object schemas in the swagger spec and add them to DB.
func AddSchemasToDB(swagger *Swagger) {
	for schemaName, schema := range swagger.Definitions {
		if _, ok := ObjDB[schemaName]; ok {
			mqutil.Logger.Printf("warning - schema %s already exists", schemaName)
		}
		ObjDB[schemaName] = &SchemaDB{schemaName, Schema(schema), nil}
	}
}

func init() {
	loads.AddLoader(fmts.YAMLMatcher, fmts.YAMLDoc)
}
