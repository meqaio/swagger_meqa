// This package handles swagger.json file parsing
package mqswag

import (
	"fmt"
	"io/ioutil"
	"log"
	"meqa/mqutil"
	"os"
	"path/filepath"
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

var MethodAll []string = []string{MethodGet, MethodPut, MethodPost, MethodDelete, MethodHead, MethodPatch, MethodOptions}

type MeqaTag struct {
	Class     string
	Property  string
	Operation string
}

func (t *MeqaTag) Equals(o *MeqaTag) bool {
	return t.Class == o.Class && t.Property == o.Property && t.Operation == o.Operation
}

func (t *MeqaTag) ToString() string {
	str := "<meqa " + t.Class
	if len(t.Property) > 0 {
		str = str + "." + t.Property
	}
	if len(t.Operation) > 0 {
		str = str + "." + t.Operation
	}
	str = str + ">"
	return str
}

// GetMeqaTag extracts the <meqa > tags.
// Example. for  <meqa Pet.Name.update>, return Pet, Name, update
func GetMeqaTag(desc string) *MeqaTag {
	if len(desc) == 0 {
		return nil
	}
	re := regexp.MustCompile("<meqa *[/-~\\-]+\\.?[/-~\\-]*\\.?[a-zA-Z]* *>")
	ar := re.FindAllString(desc, -1)

	// TODO it's possible that we have multiple choices because the server can't be
	// certain. However, we only process one right now.
	if len(ar) == 0 {
		return nil
	}
	meqa := ar[0][6:]
	right := strings.IndexRune(meqa, '>')

	if right < 0 {
		mqutil.Logger.Printf("invalid meqa tag in description: %s", desc)
		return nil
	}
	meqa = strings.Trim(meqa[:right], " ")
	contents := strings.Split(meqa, ".")
	switch len(contents) {
	case 1:
		return &MeqaTag{contents[0], "", ""}
	case 2:
		return &MeqaTag{contents[0], contents[1], ""}
	case 3:
		return &MeqaTag{contents[0], contents[1], contents[2]}
	default:
		mqutil.Logger.Printf("invalid meqa tag in description: %s", desc)
		return nil
	}
}

type Swagger spec.Swagger

// Init from a file
func CreateSwaggerFromURL(path string, meqaPath string) (*Swagger, error) {
	tmpPath := filepath.Join(meqaPath, ".meqatmp")
	os.Remove(tmpPath)
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		mqutil.Logger.Printf("can't access tmp file %s", tmpPath)
		return nil, err
	}
	defer os.Remove(tmpPath)

	// If input is yaml, transform to json
	var swaggerJsonPath string
	ar := strings.Split(path, ".")
	if ar[len(ar)-1] == "json" {
		swaggerJsonPath = path
	} else {
		yamlBytes, err := ioutil.ReadFile(path)
		if err != nil {
			mqutil.Logger.Printf("can't read file %s", path)
			return nil, err
		}
		jsonBytes, err := mqutil.YamlToJson(yamlBytes)
		if err != nil {
			mqutil.Logger.Printf("invalid yaml in file %s %v", path, err)
			return nil, err
		}
		_, err = tmpFile.Write(jsonBytes)
		if err != nil {
			mqutil.Logger.Printf("can't access tmp file %s", tmpPath)
			return nil, err
		}
		swaggerJsonPath = tmpPath
	}

	specDoc, err := loads.Spec(swaggerJsonPath)
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
	iterFunc := func(swagger *Swagger, schemaName string, schema *Schema, context map[string]interface{}) error {
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

		if len(schemaName) > 0 {
			context[schemaName] = 1
		}
		return nil
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

func CollectParamDependencies(params []spec.Parameter, swagger *Swagger, dag *DAG, tags map[string]interface{}, post bool) error {
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

func CollectResponseDependencies(responses *spec.Responses, swagger *Swagger, dag *DAG, tags map[string]interface{}, post bool) error {
	if responses == nil {
		return nil
	}
	if respSpec := responses.Default; respSpec != nil && respSpec.Schema != nil {
		err := CollectSchemaDependencies((*Schema)(respSpec.Schema), swagger, dag, tags, post)
		if err != nil {
			return err
		}
	}
	for respCode, respSpec := range responses.StatusCodeResponses {
		if respSpec.Schema != nil && respCode >= 200 && respCode < 400 {
			err := CollectSchemaDependencies((*Schema)(respSpec.Schema), swagger, dag, tags, post)
			if err != nil {
				return err
			}
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
	// are only the children for "post" (create) operations. The outputs are only the children
	// on the success code path.
	tag := GetMeqaTag(op.Description)
	creates := make(map[string]interface{})
	tags := make(map[string]interface{})
	if (tag != nil && tag.Operation == MethodPost) || (tag == nil && method == MethodPost) {
		err = CollectResponseDependencies(op.Responses, swagger, dag, creates, true)
		if err != nil {
			return err
		}
		err = CollectParamDependencies(op.Parameters, swagger, dag, creates, true)
		if err != nil {
			return err
		}
		err = AddTagsToNode(node, dag, creates, true)
		if err != nil {
			return err
		}
	}
	if (tag != nil && tag.Operation != MethodPost) || (tag == nil && method == MethodGet) {
		// For gets, we must provide some parameter in the input to get the output. It means
		// the outputs should exist on server before we make the GET call.
		err = CollectResponseDependencies(op.Responses, swagger, dag, tags, false)
		if err != nil {
			return err
		}
	}

	// This node depends on the nodes that are part of the input parameters. The input parameters
	// are parents of this node. This is the
	err = CollectParamDependencies(op.Parameters, swagger, dag, tags, false)
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
