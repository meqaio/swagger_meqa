// This package handles swagger.json file parsing
package mqswag

import (
	"fmt"
	"log"
	"meqa/mqutil"
	"regexp"
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

type MeqaTag struct {
	Class     string
	Property  string
	Operation string
}

// GetMeqaTag extracts the @meqa tags.
// Example. for  @meqa[Pet:Name].update, return Pet, Name, update
func GetMeqaTag(desc string) *MeqaTag {
	if len(desc) == 0 {
		return nil
	}
	re := regexp.MustCompile("\\@meqa\\[[a-zA-Z]*\\:?[a-zA-Z]*\\]\\.?[a-zA-Z]*")
	ar := re.FindAllString(desc, -1)

	// TODO it's possible that we have multiple choices because the server can't be
	// certain. However, we only process one right now.
	if len(ar) == 0 {
		return nil
	}
	var class, property string
	meqa := ar[0][6:]
	colon := strings.IndexRune(meqa, ':')
	right := strings.IndexRune(meqa, ']')
	if colon > 0 {
		class = meqa[:colon]
		property = meqa[colon+1 : right]
	} else {
		class = meqa[0:right]
		property = ""
	}
	if right+1 == len(meqa) {
		return &MeqaTag{class, property, ""}
	}
	return &MeqaTag{class, property, meqa[right+2:]}
}

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

func GetMethod(dagName string) string {
	return strings.Split(dagName, FieldSeparator)[2]
}

func GetDAGName(t string, n string, m string) string {
	return t + FieldSeparator + n + FieldSeparator + m
}

func AddDef(name string, schema *Schema, swagger *Swagger, dag *DAG) error {
	_, err := dag.NewNode(GetDAGName(TypeDef, name, ""), schema)
	if err != nil {
		// Name should be unique, so we don't expect this to fail.
		return err
	}
	return nil
}

// collects all the objects referred to by the schema. All the object names are put into
// the specified map.
func CollectSchemaDependencies(schema *Schema, swagger *Swagger, dag *DAG, collection map[string]interface{}, post bool) error {
	iterFunc := func(swagger *Swagger, schema *Schema, context map[string]interface{}) error {
		tag := GetMeqaTag(schema.Description)
		if tag != nil {
			// If there is a tag, and the tag's operation (which is always correct)
			// doesn't match what we want to collect, then skip.
			if post && len(tag.Operation) > 0 && tag.Operation != MethodPost {
				return nil
			}
			if !post && len(tag.Operation) > 0 && tag.Operation == MethodPost {
				return nil
			}
			if len(tag.Class) > 0 {
				context[tag.Class] = 1
			}
		}

		referenceName, _, err := swagger.GetReferredSchema((*Schema)(schema))
		if len(referenceName) > 0 {
			context[referenceName] = 1
		}
		return err
	}

	return schema.Iterate(iterFunc, collection, swagger)
}

func AddTagsToNode(node *DAGNode, dag *DAG, tags map[string]interface{}, asChild bool) error {
	var err error
	for className, _ := range tags {
		pNode := dag.NameMap[GetDAGName(TypeDef, className, "")]
		if pNode == nil {
			return mqutil.NewError(mqutil.ErrInvalid, fmt.Sprintf("tag doesn't point to a definition: %s",
				className))
		}
		if asChild {
			err = node.AddChild(pNode)
		} else {
			err = pNode.AddChild(node)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func AddParameters(params []spec.Parameter, swagger *Swagger, dag *DAG, tags map[string]interface{}, post bool) error {
	for _, param := range params {
		tag := GetMeqaTag(param.Description)
		if tag != nil && len(tag.Class) > 0 {
			// Maybe we should also add checks for whether it's in body or formData.
			if (!post && tag.Operation != MethodPost) || (post && tag.Operation == MethodPost) {
				tags[tag.Class] = 1
			}
			continue
		}

		var schema *Schema
		if param.Schema != nil {
			schema = (*Schema)(param.Schema)
		} else {
			// construct a full schema from simple ones
			schema = CreateSchemaFromSimple(&param.SimpleSchema, &param.CommonValidations)
		}
		err := CollectSchemaDependencies(schema, swagger, dag, tags, post)
		if err != nil {
			return err
		}
	}
	return nil
}

func AddOperation(pathName string, method string, op *spec.Operation, swagger *Swagger, dag *DAG) error {
	node, err := dag.NewNode(GetDAGName(TypeOp, pathName, method), op)
	if err != nil {
		return err
	}

	// The nodes that are part of outputs depends on this operation. The outputs are children.
	// We have to be careful here. Get operations will also return objects. The outputs
	// are only the children for "post" (create) operations.
	tag := GetMeqaTag(op.Description)
	creates := make(map[string]interface{})
	if (tag != nil && tag.Operation == MethodPost) || (tag == nil && method == MethodPost) {
		if op.Responses != nil {
			if respSpec := op.Responses.Default; respSpec != nil && respSpec.Schema != nil {
				err := CollectSchemaDependencies((*Schema)(respSpec.Schema), swagger, dag, creates, true)
				if err != nil {
					return err
				}
			}
			for _, respSpec := range op.Responses.StatusCodeResponses {
				if respSpec.Schema != nil {
					err := CollectSchemaDependencies((*Schema)(respSpec.Schema), swagger, dag, creates, true)
					if err != nil {
						return err
					}
				}
			}
		}
		err = AddParameters(op.Parameters, swagger, dag, creates, true)
		if err != nil {
			return err
		}
		err = AddTagsToNode(node, dag, creates, true)
		if err != nil {
			return err
		}
	}

	// This node depends on the nodes that are part of the input parameters. The input parameters
	// are parents of this node. This is the
	tags := make(map[string]interface{})
	err = AddParameters(op.Parameters, swagger, dag, tags, false)
	if err != nil {
		return err
	}
	// If we know for sure we create something, we remove it from parameters.
	for k := range creates {
		delete(tags, k)
	}
	return AddTagsToNode(node, dag, tags, false)
}

func (swagger *Swagger) AddToDAG(dag *DAG) error {
	// Add all definitions
	for name, schema := range swagger.Definitions {
		schemaCopy := Schema(schema) // must make a copy first, the schema variable is reused in the loop scope
		err := AddDef(name, &schemaCopy, swagger, dag)
		if err != nil {
			return err
		}
	}

	// Add all operations
	for pathName, pathItem := range swagger.Paths.Paths {
		if op := pathItem.Get; op != nil {
			err := AddOperation(pathName, MethodGet, op, swagger, dag)
			if err != nil {
				return err
			}
		}
		if op := pathItem.Put; op != nil {
			err := AddOperation(pathName, MethodPut, op, swagger, dag)
			if err != nil {
				return err
			}
		}
		if op := pathItem.Post; op != nil {
			err := AddOperation(pathName, MethodPost, op, swagger, dag)
			if err != nil {
				return err
			}
		}
		if op := pathItem.Delete; op != nil {
			err := AddOperation(pathName, MethodDelete, op, swagger, dag)
			if err != nil {
				return err
			}
		}
		if op := pathItem.Patch; op != nil {
			err := AddOperation(pathName, MethodPatch, op, swagger, dag)
			if err != nil {
				return err
			}
		}
		if op := pathItem.Head; op != nil {
			err := AddOperation(pathName, MethodHead, op, swagger, dag)
			if err != nil {
				return err
			}
		}
		if op := pathItem.Options; op != nil {
			err := AddOperation(pathName, MethodOptions, op, swagger, dag)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func init() {
	loads.AddLoader(fmts.YAMLMatcher, fmts.YAMLDoc)
}
