// This package handles swagger.json file parsing
package mqswag

import (
	"fmt"
	"log"
	"meqa/mqutil"
	"strings"

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

// The type code we use in DAGNode's name. e.g. a node that represents definitions/User
// will have the name of "d:User"
const (
	TypeDef        = "d"
	TypeOp         = "o"
	FieldSeparator = "?"
)

const (
	MethodGet     = "get"
	MethodPut     = "put"
	MethodPost    = "post"
	MethodDelete  = "delete"
	MethodHead    = "head"
	MethodPatch   = "patch"
	MethodOptions = "options"
)

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

// FindSchemaByName finds the schema defined by name in the swagger document.
func (swagger *Swagger) FindSchemaByName(name string) *Schema {
	schema, ok := swagger.Definitions[name]
	if !ok {
		return nil
	}
	return (*Schema)(&schema)
}

// GetReferredSchema returns what the schema refers to, and nil if it doesn't refer to any.
func (swagger *Swagger) GetReferredSchema(schema *Schema) (string, *Schema, error) {
	if schema.Ref.GetURL() == nil {
		return "", nil, nil
	}
	tokens := schema.Ref.GetPointer().DecodedTokens()
	if len(tokens) == 0 {
		return "", nil, nil
	}
	if len(tokens) != 2 || tokens[0] != "definitions" {
		return "", nil, mqutil.NewError(mqutil.ErrInvalid, fmt.Sprintf("Invalid reference: %s", schema.Ref.GetURL()))
	}
	referredSchema := swagger.FindSchemaByName(tokens[1])
	if referredSchema == nil {
		return "", nil, mqutil.NewError(mqutil.ErrInvalid, fmt.Sprintf("Reference object not found: %s", schema.Ref.GetURL()))
	}
	return tokens[1], referredSchema, nil
}

func GetType(dagName string) string {
	return dagName[0:1]
}

func GetName(dagName string) string {
	return strings.Split(dagName, FieldSeparator)[1]
}

func GetDAGName(t string, n string, method string) string {
	return t + FieldSeparator + n + FieldSeparator + method
}

func (swagger *Swagger) AddToDAG(dag *DAG) error {
	addordie := func(t string, n string, m string, d interface{}) {
		_, err := dag.NewNode(GetDAGName(t, n, m), d)
		if err != nil {
			panic(err)
		}
	}
	// Add all definitions
	for name, schema := range swagger.Definitions {
		schemaCopy := schema // must make a copy first, the schema variable is reused in the loop scope
		addordie(TypeDef, name, "", &schemaCopy)
	}

	// Add all operations
	for pathName, pathItem := range swagger.Paths.Paths {
		if op := pathItem.Get; op != nil {
			addordie(TypeOp, pathName, MethodGet, op)
		}
		if op := pathItem.Put; op != nil {
			addordie(TypeOp, pathName, MethodPut, op)
		}
		if op := pathItem.Post; op != nil {
			addordie(TypeOp, pathName, MethodPost, op)
		}
		if op := pathItem.Delete; op != nil {
			addordie(TypeOp, pathName, MethodDelete, op)
		}
		if op := pathItem.Patch; op != nil {
			addordie(TypeOp, pathName, MethodPatch, op)
		}
		if op := pathItem.Head; op != nil {
			addordie(TypeOp, pathName, MethodHead, op)
		}
		if op := pathItem.Options; op != nil {
			addordie(TypeOp, pathName, MethodOptions, op)
		}
	}
	return nil
}

func init() {
	loads.AddLoader(fmts.YAMLMatcher, fmts.YAMLDoc)
}
