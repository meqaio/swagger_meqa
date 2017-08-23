package mqplan

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"gopkg.in/resty.v0"

	"meqa/mqswag"
	"meqa/mqutil"
	"reflect"

	"encoding/json"

	"github.com/go-openapi/spec"
	"github.com/lucasjones/reggen"
	"github.com/xeipuuv/gojsonschema"
)

// The operation code in <meqa ....op> for parameters. The op code at the path level
// is the above Rest methods.
const (
	OpRead  = "read"
	OpWrite = "write"
)

// The class code in <meqa class> for responses.
const (
	ClassSuccess = "success"
	ClassFail    = "fail"
)

const (
	ExpectStatus = "status"
)

func GetBaseURL(swagger *mqswag.Swagger) string {
	// Prefer http, then https, then others.
	scheme := ""
	if len(swagger.Schemes) == 0 {
		scheme = "http"
	} else {
		for _, s := range swagger.Schemes {
			if s == "http" {
				scheme = s
				break
			} else if s == "https" {
				scheme = s
			}
		}
		if len(scheme) == 0 {
			scheme = swagger.Schemes[0]
		}
	}
	return scheme + "://" + swagger.Host + swagger.BasePath
}

// Post: old - nil, new - the new object we create.
// Put, patch: old - the old object, new - the new one.
// Get: old - the old object, new - the one we get from the server.
// Delete: old - the existing object, new - nil.
type Comparison struct {
	old    map[string]interface{} // For put and patch, it stores the keys used in lookup
	new    map[string]interface{}
	schema *spec.Schema
}

func (comp *Comparison) GetMapByOp(op string) map[string]interface{} {
	if op == OpRead {
		if comp.old == nil {
			comp.old = make(map[string]interface{})
		}
		return comp.old
	}
	if comp.new == nil {
		comp.new = make(map[string]interface{})
	}
	return comp.new
}

// Test represents a test object in the DSL. Extra care needs to be taken to copy the
// Test before running it, because running it would change the parameter maps.
type Test struct {
	Name       string                 `yaml:"name,omitempty"`
	Path       string                 `yaml:"path,omitempty"`
	Method     string                 `yaml:"method,omitempty"`
	Ref        string                 `yaml:"ref,omitempty"`
	Expect     map[string]interface{} `yaml:"expect,omitempty"`
	TestParams `yaml:",inline,omitempty" json:",inline,omitempty"`

	// Map of Object name (matching definitions) to the Comparison object.
	// This tracks what objects we need to add to DB at the end of test.
	comparisons map[string]([]*Comparison)

	tag  *mqswag.MeqaTag // The tag at the top level that describes the test
	db   *mqswag.DB
	op   *spec.Operation
	resp *resty.Response
	err  error
}

func (t *Test) Init(db *mqswag.DB) {
	t.db = db
	if len(t.Method) != 0 {
		t.Method = strings.ToLower(t.Method)
	}
	// if BodyParams is map, after unmarshal it is map[interface{}]
	var err error
	t.BodyParams, err = mqutil.YamlObjToJsonObj(t.BodyParams)
	if err != nil {
		panic(err)
	}
}

func (t *Test) Duplicate() *Test {
	test := *t
	test.Expect = mqutil.MapCopy(test.Expect)
	test.QueryParams = mqutil.MapCopy(test.QueryParams)
	test.FormParams = mqutil.MapCopy(test.FormParams)
	test.PathParams = mqutil.MapCopy(test.PathParams)
	test.HeaderParams = mqutil.MapCopy(test.HeaderParams)
	if m, ok := test.BodyParams.(map[string]interface{}); ok {
		test.BodyParams = mqutil.MapCopy(m)
	} else if a, ok := test.BodyParams.([]interface{}); ok {
		test.BodyParams = mqutil.ArrayCopy(a)
	}

	test.tag = nil
	test.op = nil
	test.resp = nil
	test.comparisons = make(map[string]([]*Comparison))
	test.err = nil

	return &test
}

func (t *Test) AddBasicComparison(tag *mqswag.MeqaTag, paramSpec *spec.Parameter, data interface{}) {
	if paramSpec == nil {
		return
	}
	if tag == nil || len(tag.Class) == 0 || len(tag.Property) == 0 {
		// No explicit tag. Info we have: t.Method, t.tag - indicate what operation we want to do.
		// t.path - indicate what object we want to operate on. We need to extrace the equivalent
		// of the tag. This is usually done on server, here we just make a simple effort.
		// TODO
		return
	}

	// It's possible that we are updating a list of objects. Due to the way we generate parameters,
	// we will always generate one complete object (both the lookup key and the new data) before we
	// move on to the next. If we find a collision, we know we need to create a new Comparison object.
	var op string
	if len(tag.Operation) > 0 {
		op = tag.Operation
	} else {
		if paramSpec.In == "formData" || paramSpec.In == "body" {
			op = OpWrite
		} else {
			op = OpRead
		}
	}
	var comp *Comparison
	if t.comparisons[tag.Class] != nil && len(t.comparisons[tag.Class]) > 0 {
		comp = t.comparisons[tag.Class][len(t.comparisons[tag.Class])-1]
		m := comp.GetMapByOp(op)
		if _, ok := m[tag.Property]; !ok {
			m[tag.Property] = data
			return
		}
	}
	// Need to create a new compare object.
	comp = &Comparison{}
	comp.schema = (*spec.Schema)(t.db.Swagger.FindSchemaByName(tag.Class))
	m := comp.GetMapByOp(op)
	m[tag.Property] = data
	t.comparisons[tag.Class] = append(t.comparisons[tag.Class], comp)
}

func (t *Test) AddObjectComparison(class string, method string, obj map[string]interface{}, schema *spec.Schema) {
	if method == mqswag.MethodPost {
		t.comparisons[class] = append(t.comparisons[class], &Comparison{nil, obj, schema})
	} else if method == mqswag.MethodPut || method == mqswag.MethodPatch {
		// It's possible that we are updating a list of objects. Due to the way we generate parameters,
		// we will always generate one complete object (both the lookup key and the new data) before we
		// move on to the next.
		if t.comparisons[class] != nil && len(t.comparisons[class]) > 0 {
			last := t.comparisons[class][len(t.comparisons[class])-1]
			if last.new == nil {
				last.new = obj
				return
			}
			// During put, having an array of objects with just the "new" part is allowed. This
			// means the update key is included in the new object.
		}
		t.comparisons[class] = append(t.comparisons[class], &Comparison{nil, obj, schema})
	} else {
		mqutil.Logger.Printf("unexpected: generating object %s for GET method.", class)
	}
}

// ProcessOneComparison processes one comparison object.
func (t *Test) ProcessOneComparison(className string, comp *Comparison, resultArray []interface{}) error {
	method := t.Method
	if t.tag != nil && len(t.tag.Operation) > 0 {
		method = t.tag.Operation
	}

	if method == mqswag.MethodGet {
		var matchFunc mqswag.MatchFunc
		if comp.old == nil {
			matchFunc = mqswag.MatchAlways
		} else {
			matchFunc = mqswag.MatchAllFields
		}
		dbArray := t.db.Find(className, comp.old, matchFunc, -1)
		// What we found from the server (resultArray) and from in-memory DB using the same criteria should match.
		if len(resultArray) != len(dbArray) {
			resultBytes, _ := json.Marshal(resultArray)
			mqutil.Logger.Print(string(resultBytes))
			return mqutil.NewError(mqutil.ErrHttp, fmt.Sprintf("expecting %d entries got %d entries",
				len(dbArray), len(resultArray)))
		}

		// TODO optimize later. Should sort first.
		for _, entry := range resultArray {
			found := false
			entryMap, _ := entry.(map[string]interface{})
			if entryMap == nil {
				if len(dbArray) == 0 {
					// Server returned array of non-map types. The db shouldn't expect anything.
					continue
				}
			} else {
				for _, dbEntry := range dbArray {
					dbentryMap, _ := dbEntry.(map[string]interface{})
					if dbentryMap != nil && mqutil.MapEquals(entryMap, dbentryMap, false) {
						found = true
						break
					}
				}
			}
			if !found {
				b, _ := json.Marshal(entry)
				return mqutil.NewError(mqutil.ErrHttp, fmt.Sprintf("result returned is not found on client\n%s\n",
					string(b)))
			}
		}
	} else if method == mqswag.MethodDelete {
		t.db.Delete(className, comp.old, mqswag.MatchAllFields, -1)
	} else if method == mqswag.MethodPost && comp.new != nil {
		return t.db.Insert(className, comp.schema, comp.new)
	} else if (method == mqswag.MethodPatch || method == mqswag.MethodPut) && comp.new != nil {
		count := t.db.Update(className, comp.old, mqswag.MatchAllFields, comp.new, 1, method == mqswag.MethodPatch)
		if count != 1 {
			mqutil.Logger.Printf("Failed to find any entry to update")
		}
	}
	return nil
}

func (t *Test) GetParamFromComparison(name string, where string) interface{} {
	for _, compList := range t.comparisons {
		for _, comp := range compList {
			if v := comp.new[name]; v != nil && (where == "any" || where == "new") {
				return v
			}
			if v := comp.old[name]; v != nil && (where == "any" || where == "old") {
				return v
			}
		}
	}
	return nil
}

// ProcessResult decodes the response from the server into a result array
func (t *Test) ProcessResult(resp *resty.Response) error {
	t.resp = resp
	status := resp.StatusCode()
	var respSpec *spec.Response
	if t.op.Responses != nil {
		respObject, ok := t.op.Responses.StatusCodeResponses[status]
		if ok {
			respSpec = &respObject
		} else {
			respSpec = t.op.Responses.Default
		}
	}
	if respSpec == nil {
		// Nothing specified in the swagger.json. Same as an empty spec.
		respSpec = &spec.Response{}
	}

	respBody := resp.Body()
	// Check if the response obj and respSchema match
	respSchema := (*mqswag.Schema)(respSpec.Schema)
	var resultObj interface{}

	if respSchema != nil {
		if len(respBody) > 0 {
			err := json.Unmarshal(respBody, &resultObj)
			if err == nil {
				if !respSchema.Matches(resultObj, t.db.Swagger) {
					return mqutil.NewError(mqutil.ErrServerResp, fmt.Sprintf("server response doesn't match swagger spec: %s", string(respBody)))
				}
			} else if !respSchema.Type.Contains(gojsonschema.TYPE_STRING) {
				return mqutil.NewError(mqutil.ErrServerResp, fmt.Sprintf("server response doesn't match swagger spec: %s", string(respBody)))
			}
		} else {
			// If schema is an array, then not having a body is OK
			if !respSchema.Type.Contains(gojsonschema.TYPE_ARRAY) {
				return mqutil.NewError(mqutil.ErrServerResp, fmt.Sprintf("swagger.spec expects a non-empty response, but response body is actually empty"))
			}
		}

		// Remove the comparison objects that 1) swagger doesn't expect back 2) we didn't modify
		for className, compList := range t.comparisons {
			count := 0
			for i, comp := range compList {
				if comp.new == nil && !respSchema.Contains(className, t.db.Swagger) {
					compList[i] = compList[count]
					count++
				}
			}
			t.comparisons[className] = compList[count:]
		}
	}

	// success based on return status
	success := (status >= 200 && status < 300)
	tag := mqswag.GetMeqaTag(respSpec.Description)
	if tag != nil && tag.Class == ClassFail {
		success = false
	}

	if t.Expect != nil && t.Expect[ExpectStatus] != nil {
		expectedStatus := t.Expect[ExpectStatus]
		if expectedStatus == "fail" {
			success = !success
		} else if expectedStatusNum, ok := expectedStatus.(int); ok {
			success = (expectedStatusNum == status)
		} else {
			success = false
		}
	}

	if !success {
		actuallyFailed := true
		if actuallyFailed {
			mqutil.Logger.Printf("=== test failed, response code %d ===", status)
		}
		return nil
	}

	var resultArray []interface{}
	var ok bool
	if resultObj != nil {
		if resultArray, ok = resultObj.([]interface{}); !ok {
			resultArray = []interface{}{resultObj}
		}
	}
	// Success, replace or verify based on method.
	for className, compArray := range t.comparisons {
		for _, c := range compArray {
			err := t.ProcessOneComparison(className, c, resultArray)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// SetRequestParameters sets the parameters. Returns the new request path.
func (t *Test) SetRequestParameters(req *resty.Request) string {
	if len(t.QueryParams) > 0 {
		req.SetQueryParams(mqutil.MapInterfaceToMapString(t.QueryParams))
		mqutil.InterfacePrint(t.QueryParams, "queryParams:\n")
	}
	if t.BodyParams != nil {
		req.SetBody(t.BodyParams)
		mqutil.InterfacePrint(t.BodyParams, "bodyParams:\n")
	}
	if len(t.HeaderParams) > 0 {
		req.SetHeaders(mqutil.MapInterfaceToMapString(t.HeaderParams))
		mqutil.InterfacePrint(t.HeaderParams, "headerParams:\n")
	}
	if len(t.FormParams) > 0 {
		req.SetFormData(mqutil.MapInterfaceToMapString(t.FormParams))
		mqutil.InterfacePrint(t.FormParams, "formParams:\n")
	}
	path := t.Path
	if len(t.PathParams) > 0 {
		PathParamsStr := mqutil.MapInterfaceToMapString(t.PathParams)
		for k, v := range PathParamsStr {
			path = strings.Replace(path, "{"+k+"}", v, -1)
		}
		mqutil.InterfacePrint(t.PathParams, "pathParams:\n")
	}
	return path
}

func (t *Test) CopyParent(parentTest *Test) {
	if parentTest != nil {
		t.Expect = mqutil.MapCopy(parentTest.Expect)
		t.QueryParams = mqutil.MapAdd(t.QueryParams, parentTest.QueryParams)
		t.PathParams = mqutil.MapAdd(t.PathParams, parentTest.PathParams)
		t.HeaderParams = mqutil.MapAdd(t.HeaderParams, parentTest.HeaderParams)
		t.FormParams = mqutil.MapAdd(t.FormParams, parentTest.FormParams)

		if parentTest.BodyParams != nil {
			if t.BodyParams == nil {
				t.BodyParams = parentTest.BodyParams
			} else {
				// replace with parent only if the types are the same
				if parentBodyMap, ok := parentTest.BodyParams.(map[string]interface{}); ok {
					if bodyMap, ok := t.BodyParams.(map[string]interface{}); ok {
						t.BodyParams = mqutil.MapCombine(bodyMap, parentBodyMap)
					}
				} else {
					// For non-map types, just replace with parent if they are the same type.
					if reflect.TypeOf(parentTest.BodyParams) == reflect.TypeOf(t.BodyParams) {
						t.BodyParams = parentTest.BodyParams
					}
				}
			}
		}
	}
}

// Run runs the test. Returns the test result.
func (t *Test) Run(tc *TestCase) error {

	mqutil.Logger.Print("\n--- " + t.Name)
	err := t.ResolveParameters(tc)
	if err != nil {
		return err
	}

	req := resty.R()
	if len(tc.Username) > 0 {
		req.SetBasicAuth(tc.Username, tc.Password)
	}
	path := GetBaseURL(t.db.Swagger) + t.SetRequestParameters(req)
	var resp *resty.Response

	switch t.Method {
	case mqswag.MethodGet:
		resp, err = req.Get(path)
	case mqswag.MethodPost:
		resp, err = req.Post(path)
	case mqswag.MethodPut:
		resp, err = req.Put(path)
	case mqswag.MethodDelete:
		resp, err = req.Delete(path)
	case mqswag.MethodPatch:
		resp, err = req.Patch(path)
	case mqswag.MethodHead:
		resp, err = req.Head(path)
	case mqswag.MethodOptions:
		resp, err = req.Options(path)
	default:
		return mqutil.NewError(mqutil.ErrInvalid, fmt.Sprintf("Unknown method in test %s: %v", t.Name, t.Method))
	}
	if err != nil {
		return mqutil.NewError(mqutil.ErrHttp, err.Error())
	}
	// TODO properly process resp. Check against the current DB to see if they match
	mqutil.Logger.Print(resp.Status())
	mqutil.Logger.Println(string(resp.Body()))
	return t.ProcessResult(resp)
}

// GetSchemaRootType gets the real object type fo the specified schema. It only returns meaningful
// data for object and array of object type of parameters. If the parameter is a basic type it returns
// nil
func (t *Test) GetSchemaRootType(schema *mqswag.Schema, parentTag *mqswag.MeqaTag) (*mqswag.MeqaTag, *mqswag.Schema) {
	tag := mqswag.GetMeqaTag(schema.Description)
	if tag == nil {
		tag = parentTag
	}
	referenceName, referredSchema, err := t.db.Swagger.GetReferredSchema((*mqswag.Schema)(schema))
	if err != nil {
		mqutil.Logger.Print(err)
		return nil, nil
	}
	if referredSchema != nil {
		if tag == nil {
			tag = &mqswag.MeqaTag{referenceName, "", ""}
		}
		return t.GetSchemaRootType(referredSchema, tag)
	}
	if len(schema.Enum) != 0 {
		return nil, nil
	}
	if len(schema.Type) == 0 {
		return nil, nil
	}
	if schema.Type.Contains(gojsonschema.TYPE_ARRAY) {
		var itemSchema *spec.Schema
		if len(schema.Items.Schemas) != 0 {
			itemSchema = &(schema.Items.Schemas[0])
		} else {
			itemSchema = schema.Items.Schema
		}
		return t.GetSchemaRootType((*mqswag.Schema)(itemSchema), tag)
	} else if schema.Type.Contains(gojsonschema.TYPE_OBJECT) {
		return tag, schema
	}
	return nil, nil
}

func StringParamsResolveWithHistory(str string, h *TestHistory) interface{} {
	begin := strings.Index(str, "<")
	end := strings.Index(str, ">")
	if end > begin {
		ar := strings.Split(str[begin+1:end], ".")
		if len(ar) != 2 {
			mqutil.Logger.Printf("invalid parameter: %s", str[begin+1:end])
			return nil
		}
		t := h.GetTest(ar[0])
		if t != nil {
			return t.GetParamFromComparison(ar[1], "any")
		}
	}
	return nil
}

func MapParamsResolveWithHistory(paramMap map[string]interface{}, h *TestHistory) {
	for k, v := range paramMap {
		if str, ok := v.(string); ok {
			if result := StringParamsResolveWithHistory(str, h); result != nil {
				paramMap[k] = result
			}
		}
	}
}

func ArrayParamsResolveWithHistory(paramArray []interface{}, h *TestHistory) {
	for i, param := range paramArray {
		if paramMap, ok := param.(map[string]interface{}); ok {
			MapParamsResolveWithHistory(paramMap, h)
		} else if str, ok := param.(string); ok {
			if result := StringParamsResolveWithHistory(str, h); result != nil {
				paramArray[i] = result
			}
		}
	}
}

func (t *Test) ResolveHistoryParameters(h *TestHistory) {
	MapParamsResolveWithHistory(t.PathParams, h)
	MapParamsResolveWithHistory(t.FormParams, h)
	MapParamsResolveWithHistory(t.HeaderParams, h)
	MapParamsResolveWithHistory(t.QueryParams, h)
	if bodyMap, ok := t.BodyParams.(map[string]interface{}); ok {
		MapParamsResolveWithHistory(bodyMap, h)
	} else if bodyArray, ok := t.BodyParams.([]interface{}); ok {
		ArrayParamsResolveWithHistory(bodyArray, h)
	} else if bodyStr, ok := t.BodyParams.(string); ok {
		result := StringParamsResolveWithHistory(bodyStr, h)
		if result != nil {
			t.BodyParams = result
		}
	}
}

// ParamsAdd adds the parameters from src to dst if the param doesn't already exist on dst.
func ParamsAdd(dst []spec.Parameter, src []spec.Parameter) []spec.Parameter {
	if len(dst) == 0 {
		return src
	}
	if len(src) == 0 {
		return dst
	}
	nameMap := make(map[string]int)
	for _, entry := range dst {
		nameMap[entry.Name] = 1
	}
	for _, entry := range src {
		if nameMap[entry.Name] != 1 {
			dst = append(dst, entry)
			nameMap[entry.Name] = 1
		}
	}
	return dst
}

// ResolveParameters fullfills the parameters for the specified request using the in-mem DB.
// The resolved parameters will be added to test.Parameters map.
func (t *Test) ResolveParameters(tc *TestCase) error {
	pathItem := t.db.Swagger.Paths.Paths[t.Path]
	t.op = GetOperationByMethod(&pathItem, t.Method)
	if t.op == nil {
		return mqutil.NewError(mqutil.ErrNotFound, fmt.Sprintf("Path %s not found in swagger file", t.Path))
	}

	// There can be parameters at the path level. We merge these with the operation parameters.
	t.op.Parameters = ParamsAdd(t.op.Parameters, pathItem.Parameters)

	t.tag = mqswag.GetMeqaTag(t.op.Description)

	var paramsMap map[string]interface{}
	var globalParamsMap map[string]interface{}
	var err error
	var genParam interface{}
	for _, params := range t.op.Parameters {
		if params.In == "body" {
			var bodyMap map[string]interface{}
			bodyIsMap := false
			if t.BodyParams != nil {
				bodyMap, bodyIsMap = t.BodyParams.(map[string]interface{})
			}
			if t.BodyParams != nil && !bodyIsMap {
				// Body is not map, we use it directly.
				objarray := mqutil.InterfaceToArray(t.BodyParams)
				paramTag, schema := t.GetSchemaRootType((*mqswag.Schema)(params.Schema), mqswag.GetMeqaTag(params.Description))
				method := t.Method
				if t.tag != nil && len(t.tag.Operation) > 0 {
					method = t.tag.Operation
				}
				if schema != nil && paramTag != nil {
					for _, obj := range objarray {
						t.AddObjectComparison(paramTag.Class, method, obj, (*spec.Schema)(schema))
					}
				}
				continue
			}
			// Body is map, we generate parameters, then use the value in the original t and tc's bodyParam where possible
			genParam, err = t.GenerateParameter(&params, t.db)
			if err != nil {
				return err
			}
			if genMap, genIsMap := genParam.(map[string]interface{}); genIsMap {
				if tcBodyMap, tcIsMap := tc.BodyParams.(map[string]interface{}); tcIsMap {
					bodyMap = mqutil.MapAdd(bodyMap, tcBodyMap)
				}
				t.BodyParams = mqutil.MapReplace(genMap, bodyMap)
			} else {
				t.BodyParams = genParam
			}
		} else {
			switch params.In {
			case "path":
				if t.PathParams == nil {
					t.PathParams = make(map[string]interface{})
				}
				paramsMap = t.PathParams
				globalParamsMap = tc.PathParams
			case "query":
				if t.QueryParams == nil {
					t.QueryParams = make(map[string]interface{})
				}
				paramsMap = t.QueryParams
				globalParamsMap = tc.QueryParams
			case "header":
				if t.HeaderParams == nil {
					t.HeaderParams = make(map[string]interface{})
				}
				paramsMap = t.HeaderParams
				globalParamsMap = tc.HeaderParams
			case "formData":
				if t.FormParams == nil {
					t.FormParams = make(map[string]interface{})
				}
				paramsMap = t.FormParams
				globalParamsMap = tc.FormParams
			}

			// If there is a parameter passed in, just use it. Otherwise generate one.
			if paramsMap[params.Name] == nil && globalParamsMap[params.Name] != nil {
				paramsMap[params.Name] = globalParamsMap[params.Name]
			}
			if _, ok := paramsMap[params.Name]; ok {
				t.AddBasicComparison(mqswag.GetMeqaTag(params.Description), &params, paramsMap[params.Name])
				continue
			}
			genParam, err = t.GenerateParameter(&params, t.db)
			paramsMap[params.Name] = genParam
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func GetOperationByMethod(item *spec.PathItem, method string) *spec.Operation {
	switch method {
	case mqswag.MethodGet:
		return item.Get
	case mqswag.MethodPost:
		return item.Post
	case mqswag.MethodPut:
		return item.Put
	case mqswag.MethodDelete:
		return item.Delete
	case mqswag.MethodPatch:
		return item.Patch
	case mqswag.MethodHead:
		return item.Head
	case mqswag.MethodOptions:
		return item.Options
	}
	return nil
}

// GenerateParameter generates paramter value based on the spec.
func (t *Test) GenerateParameter(paramSpec *spec.Parameter, db *mqswag.DB) (interface{}, error) {
	tag := mqswag.GetMeqaTag(paramSpec.Description)
	if paramSpec.Schema != nil {
		return t.GenerateSchema(paramSpec.Name, tag, paramSpec.Schema, db)
	}
	if len(paramSpec.Enum) != 0 {
		return generateEnum(paramSpec.Enum)
	}
	if len(paramSpec.Type) == 0 {
		return nil, mqutil.NewError(mqutil.ErrInvalid, "Parameter doesn't have type")
	}

	var schema *spec.Schema
	if paramSpec.Schema != nil {
		schema = paramSpec.Schema
	} else {
		// construct a full schema from simple ones
		schema = (*spec.Schema)(mqswag.CreateSchemaFromSimple(&paramSpec.SimpleSchema, &paramSpec.CommonValidations))
	}
	if paramSpec.Type == gojsonschema.TYPE_OBJECT {
		return t.generateObject("param_", tag, schema, db)
	}
	if paramSpec.Type == gojsonschema.TYPE_ARRAY {
		return t.generateArray("param_", tag, schema, db)
	}

	return t.generateByType(schema, paramSpec.Name+"_", tag, paramSpec)
}

// Two ways to get to generateByType
// 1) directly called from GenerateParameter, now we know the type is a parameter, and we want to add to comparison
// 2) called at bottom level, here we know the object will be added to comparison and not the type primitives.
func (t *Test) generateByType(s *spec.Schema, prefix string, parentTag *mqswag.MeqaTag, paramSpec *spec.Parameter) (interface{}, error) {
	tag := mqswag.GetMeqaTag(s.Description)
	if tag == nil {
		tag = parentTag
	}
	if paramSpec != nil {
		if tag != nil && len(tag.Property) > 0 {
			// Try to get one from the comparison objects.
			for _, c := range t.comparisons[tag.Class] {
				if c.old != nil {
					return c.old[tag.Property], nil
				}
			}
			// Get one from in-mem db and populate the comparison structure.
			ar := t.db.Find(tag.Class, nil, mqswag.MatchAlways, 5)
			if len(ar) > 0 {
				obj := ar[rand.Intn(len(ar))].(map[string]interface{})
				comp := &Comparison{obj, nil, (*spec.Schema)(t.db.GetSchema(tag.Class))}
				t.comparisons[tag.Class] = append(t.comparisons[tag.Class], comp)
				return obj[tag.Property], nil
			}
		}
	}

	if len(s.Type) != 0 {
		var result interface{}
		var err error
		switch s.Type[0] {
		case gojsonschema.TYPE_BOOLEAN:
			result, err = generateBool(s)
		case gojsonschema.TYPE_INTEGER:
			result, err = generateInt(s)
		case gojsonschema.TYPE_NUMBER:
			result, err = generateFloat(s)
		case gojsonschema.TYPE_STRING:
			result, err = generateString(s, prefix)
		case "file":
			result, err = reggen.Generate("[0-9]+", 200)
		}
		if result != nil && err == nil {
			t.AddBasicComparison(tag, paramSpec, result)
			return result, err
		}
	}

	return nil, mqutil.NewError(mqutil.ErrInvalid, fmt.Sprintf("unrecognized type: %s", s.Type))
}

// RandomTime generate a random time in the range of [t - r, t).
func RandomTime(t time.Time, r time.Duration) time.Time {
	return t.Add(-time.Duration(float64(r) * rand.Float64()))
}

// TODO we need to make it context aware. Based on different contexts we should generate different
// date ranges. Prefix is a prefix to use when generating strings. It's only used when there is
// no specified pattern in the swagger.json
func generateString(s *spec.Schema, prefix string) (string, error) {
	if s.Format == "date-time" {
		t := RandomTime(time.Now(), time.Hour*24*30)
		return t.Format(time.RFC3339), nil
	}
	if s.Format == "date" {
		t := RandomTime(time.Now(), time.Hour*24*30)
		return t.Format("2006-01-02"), nil
	}

	// If no pattern is specified, we use the field name + some numbers as pattern
	var pattern string
	length := 0
	if len(s.Pattern) != 0 {
		pattern = s.Pattern
		length = len(s.Pattern) * 2
	} else {
		pattern = prefix + "\\d+"
		length = len(prefix) + 5
	}
	str, err := reggen.Generate(pattern, length)
	if err != nil {
		return "", mqutil.NewError(mqutil.ErrInvalid, err.Error())
	}

	if len(s.Format) == 0 || s.Format == "password" {
		return str, nil
	}
	if s.Format == "byte" {
		return base64.StdEncoding.EncodeToString([]byte(str)), nil
	}
	if s.Format == "binary" {
		return hex.EncodeToString([]byte(str)), nil
	}
	if s.Format == "uri" || s.Format == "url" {
		return "https://www.google.com/search?q=" + str, nil
	}
	return "", mqutil.NewError(mqutil.ErrInvalid, fmt.Sprintf("Invalid format string: %s", s.Format))
}

func generateBool(s *spec.Schema) (interface{}, error) {
	return rand.Intn(2) == 0, nil
}

func generateFloat(s *spec.Schema) (float64, error) {
	var realmin float64
	if s.Minimum != nil {
		realmin = *s.Minimum
		if s.ExclusiveMinimum {
			realmin += 0.01
		}
	}
	var realmax float64
	if s.Maximum != nil {
		realmax = *s.Maximum
		if s.ExclusiveMaximum {
			realmax -= 0.01
		}
	}
	if realmin >= realmax {
		if s.Minimum == nil && s.Maximum == nil {
			realmin = -1.0
			realmax = 1.0
		} else if s.Minimum != nil {
			realmax = realmin + math.Abs(realmin)
		} else if s.Maximum != nil {
			realmin = realmax - math.Abs(realmax)
		} else {
			// both are present but conflicting
			return 0, mqutil.NewError(mqutil.ErrInvalid, fmt.Sprintf("specified min value %v is bigger than max %v",
				*s.Minimum, *s.Maximum))
		}
	}
	return rand.Float64()*(realmax-realmin) + realmin, nil
}

func generateInt(s *spec.Schema) (int64, error) {
	// Give a default range if there isn't one
	if s.Maximum == nil && s.Minimum == nil {
		maxf := 10000.0
		s.Maximum = &maxf
	}
	f, err := generateFloat(s)
	if err != nil {
		return 0, err
	}
	i := int64(f)
	if s.Minimum != nil && i <= int64(*s.Minimum) {
		i++
	}
	return i, nil
}

func (t *Test) generateArray(name string, parentTag *mqswag.MeqaTag, schema *spec.Schema, db *mqswag.DB) (interface{}, error) {
	var numItems int
	if schema.MaxItems != nil || schema.MinItems != nil {
		var maxItems int = 10
		if schema.MaxItems != nil {
			maxItems = int(*schema.MaxItems)
			if maxItems < 0 {
				maxItems = 0
			}
		}
		var minItems int
		if schema.MinItems != nil {
			minItems = int(*schema.MinItems)
			if minItems < 0 {
				minItems = 0
			}
		}
		maxDiff := maxItems - minItems
		if maxDiff <= 0 {
			maxDiff = 1
		}
		numItems = rand.Intn(int(maxDiff)) + minItems
	} else {
		numItems = rand.Intn(10)
	}

	var itemSchema *spec.Schema
	if len(schema.Items.Schemas) != 0 {
		itemSchema = &(schema.Items.Schemas[0])
	} else {
		itemSchema = schema.Items.Schema
	}
	tag := mqswag.GetMeqaTag(schema.Description)
	if tag == nil {
		tag = parentTag
	}

	var ar []interface{}
	var hash map[interface{}]interface{}
	if schema.UniqueItems {
		hash = make(map[interface{}]interface{})
	}

	for i := 0; i < numItems; i++ {
		entry, err := t.GenerateSchema(name, tag, itemSchema, db)
		if err != nil {
			return nil, err
		}
		if hash != nil && hash[entry] != nil {
			continue
		}
		ar = append(ar, entry)
		if hash != nil {
			hash[entry] = 1
		}
	}
	return ar, nil
}

func (t *Test) generateObject(name string, parentTag *mqswag.MeqaTag, schema *spec.Schema, db *mqswag.DB) (interface{}, error) {
	obj := make(map[string]interface{})
	for k, v := range schema.Properties {
		propertyTag := mqswag.GetMeqaTag(v.Description)
		if propertyTag == nil {
			propertyTag = parentTag
		}
		o, err := t.GenerateSchema(k+"_", propertyTag, &v, db)
		if err != nil {
			return nil, err
		}
		obj[k] = o
	}

	tag := mqswag.GetMeqaTag(schema.Description)
	if tag == nil {
		tag = parentTag
	}
	var class, method string
	if tag != nil {
		class = tag.Class
	}
	if t.tag != nil && len(t.tag.Operation) > 0 {
		method = t.tag.Operation // At test level the tag indicates the real method
	} else {
		method = t.Method
	}
	if len(class) == 0 {
		cl, s := db.FindMatchingSchema(obj)
		if s == nil {
			mqutil.Logger.Printf("Can't find a known schema for obj %s", name)
			return obj, nil
		}
		class = cl
	}

	t.AddObjectComparison(class, method, obj, schema)
	return obj, nil
}

func (t *Test) GenerateSchema(name string, tag *mqswag.MeqaTag, schema *spec.Schema, db *mqswag.DB) (interface{}, error) {
	swagger := db.Swagger
	// Deal with refs.
	referenceName, referredSchema, err := swagger.GetReferredSchema((*mqswag.Schema)(schema))
	if err != nil {
		return nil, err
	}
	if referredSchema != nil {
		var paramTag mqswag.MeqaTag
		if tag != nil {
			paramTag = mqswag.MeqaTag{referenceName, tag.Property, tag.Operation}
		} else {
			paramTag = mqswag.MeqaTag{referenceName, "", ""}
		}
		return t.GenerateSchema(name, &paramTag, (*spec.Schema)(referredSchema), db)
	}

	if len(schema.Enum) != 0 {
		return generateEnum(schema.Enum)
	}

	if len(schema.AllOf) > 0 {
		combined := make(map[string]interface{})
		for _, s := range schema.AllOf {
			m, err := t.GenerateSchema(name, tag, &s, db)
			if err != nil {
				return nil, err
			}
			if o, isMap := m.(map[string]interface{}); isMap {
				combined = mqutil.MapCombine(combined, o)
			} else {
				// We don't know how to combine AllOf properties of non-map types.
				jsonStr, _ := json.MarshalIndent(schema, "", "    ")
				return nil, mqutil.NewError(mqutil.ErrInvalid, fmt.Sprintf("can't combine AllOf schema that's not map: %s", jsonStr))
			}
		}
		return combined, nil
	}

	if len(schema.Type) == 0 {
		return nil, mqutil.NewError(mqutil.ErrInvalid, "Parameter doesn't have type")
	}
	if schema.Type[0] == gojsonschema.TYPE_OBJECT {
		return t.generateObject(name, tag, schema, db)
	}
	if schema.Type[0] == gojsonschema.TYPE_ARRAY {
		return t.generateArray(name, tag, schema, db)
	}

	return t.generateByType(schema, name, tag, nil)
}

func generateEnum(e []interface{}) (interface{}, error) {
	return e[rand.Intn(len(e))], nil
}
