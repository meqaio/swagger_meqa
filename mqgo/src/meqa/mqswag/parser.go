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

// The class code in <meqa class> for responses.
const (
	ClassSuccess = "success"
	ClassFail    = "fail"
	ClassWeak    = "weak"
)

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

// Dependencies keeps track of what this operation consumes and produces. It also keeps
// track of what the default dependency is when there is no tag. Default always point to
// either "Produces" or "Consumes"
type Dependencies struct {
	Produces map[string]interface{}
	Consumes map[string]interface{}
	Default  map[string]interface{}
	IsPost   bool
}

// CollectFromTag collects from the tag. It returns true if successful, false if not.
func (dep *Dependencies) CollectFromTag(tag *MeqaTag) bool {
	if tag != nil && len(tag.Class) > 0 {
		// If there is a tag, and the tag's operation (which is always correct)
		// doesn't match what we want to collect, then skip.
		if len(tag.Operation) > 0 {
			if tag.Operation == MethodPost {
				dep.Produces[tag.Class] = 1
			} else {
				dep.Consumes[tag.Class] = 1
			}
		} else {
			dep.Default[tag.Class] = 1
		}
		return true
	}
	return false
}

// collects all the objects referred to by the schema. All the object names are put into
// the specified map.
func CollectSchemaDependencies(schema *Schema, swagger *Swagger, dag *DAG, dep *Dependencies) error {
	iterFunc := func(swagger *Swagger, schemaName string, schema *Schema, context interface{}) error {
		collected := dep.CollectFromTag(GetMeqaTag(schema.Description))
		if !collected && len(schemaName) > 0 {
			dep.Default[schemaName] = 1
		}

		return nil
	}

	return schema.Iterate(iterFunc, dep, swagger, false)
}

func CollectParamDependencies(params []spec.Parameter, swagger *Swagger, dag *DAG, dep *Dependencies) error {
	defer func() { dep.Default = nil }()
	for _, param := range params {
		if dep.IsPost && (param.In == "body" || param.In == "formData") {
			dep.Default = dep.Produces
		} else {
			dep.Default = dep.Consumes
		}

		tag := GetMeqaTag(param.Description)
		collected := dep.CollectFromTag(tag)
		if collected {
			continue
		}

		var schema *Schema
		if param.Schema != nil {
			schema = (*Schema)(param.Schema)
		} else {
			// construct a full schema from simple ones
			schema = CreateSchemaFromSimple(&param.SimpleSchema, &param.CommonValidations)
		}

		err := CollectSchemaDependencies(schema, swagger, dag, dep)
		if err != nil {
			return err
		}
	}

	// This heuristics is for the common case. When posting an object, the fields referred by it are input
	// parameters needed to create this object. More complicated cases are to be handled by tags.
	for name := range dep.Produces {
		schema := swagger.FindSchemaByName(name)
		dep.Default = dep.Consumes

		err := CollectSchemaDependencies(schema, swagger, dag, dep)
		if err != nil {
			return err
		}
	}

	return nil
}

func CollectResponseDependencies(responses *spec.Responses, swagger *Swagger, dag *DAG, dep *Dependencies) error {
	if responses == nil {
		return nil
	}
	dep.Default = make(map[string]interface{}) // We don't assume by default anything so we throw Default away.
	defer func() { dep.Default = nil }()
	for respCode, respSpec := range responses.StatusCodeResponses {
		collected := dep.CollectFromTag(GetMeqaTag(respSpec.Description))
		if collected {
			continue
		}
		if respSpec.Schema != nil && respCode >= 200 && respCode < 300 {
			err := CollectSchemaDependencies((*Schema)(respSpec.Schema), swagger, dag, dep)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func AddOperation(pathName string, pathItem *spec.PathItem, method string, swagger *Swagger, dag *DAG, setPriority bool) error {
	opInterface, err := pathItem.JSONLookup(method)
	if err != nil {
		return err
	}
	op := opInterface.(*spec.Operation)
	if op == nil {
		return nil
	}

	var node *DAGNode
	if setPriority {
		node = dag.NameMap[GetDAGName(TypeOp, pathName, method)]
	} else {
		node, err = dag.NewNode(GetDAGName(TypeOp, pathName, method), op)
		if err != nil {
			return err
		}
	}

	// The nodes that are part of outputs depends on this operation. The outputs are children.
	// We have to be careful here. Get operations will also return objects. For gets, the outputs
	// are children only if they are not part of input parameters.
	tag := GetMeqaTag(op.Description)
	dep := &Dependencies{}
	dep.Produces = make(map[string]interface{})
	dep.Consumes = make(map[string]interface{})

	if (tag != nil && tag.Operation == MethodPost) || (tag == nil && method == MethodPost) {
		dep.IsPost = true
	} else {
		dep.IsPost = false
	}

	// The order matters. At the end of CollectParamDependencies we collect the parameters
	// referred by the object we produce.
	err = CollectParamDependencies(op.Parameters, swagger, dag, dep)
	if err != nil {
		return err
	}

	err = CollectParamDependencies(pathItem.Parameters, swagger, dag, dep)
	if err != nil {
		return err
	}

	err = CollectResponseDependencies(op.Responses, swagger, dag, dep)
	if err != nil {
		return err
	}

	// Get the highest parameter weight before we remove circular dependencies.
	if setPriority {
		for consumeName := range dep.Consumes {
			paramNode := dag.NameMap[GetDAGName(TypeDef, consumeName, "")]
			if node.Priority < paramNode.Weight {
				node.Priority = paramNode.Weight
			}
		}
		return nil
	}

	if dep.IsPost {
		// We are creating object. Some of the inputs will be from the same object, remove them from
		// the consumes field.
		for k := range dep.Produces {
			delete(dep.Consumes, k)
		}
	} else {
		// We are getting objects. We definitely depend on the parameters we consume.
		for k := range dep.Consumes {
			delete(dep.Produces, k)
		}
	}

	err = node.AddDependencies(dag, dep.Produces, true)
	if err != nil {
		return err
	}

	return node.AddDependencies(dag, dep.Consumes, false)
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
	// Add all children
	for name, schema := range swagger.Definitions {
		node := dag.NameMap[GetDAGName(TypeDef, name, "")]
		collections := make(map[string]interface{})
		collectInner := func(swagger *Swagger, schemaName string, schema *Schema, context interface{}) error {
			if len(schemaName) > 0 && schemaName != name {
				collections[schemaName] = 1
			}
			return nil
		}
		((*Schema)(&schema)).Iterate(collectInner, nil, swagger, false)
		// The inner fields are the parents. The child depends on parents.
		node.AddDependencies(dag, collections, false)
	}

	// Add all operations
	for pathName, pathItem := range swagger.Paths.Paths {
		for _, method := range MethodAll {
			err := AddOperation(pathName, &pathItem, method, swagger, dag, false)
			if err != nil {
				return err
			}
		}
	}
	// set priorities. This can only be done after the above, where all weights for all operations are set.
	for pathName, pathItem := range swagger.Paths.Paths {
		for _, method := range MethodAll {
			err := AddOperation(pathName, &pathItem, method, swagger, dag, true)
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
