package mqplan

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
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
	uuid "github.com/satori/go.uuid"
	"github.com/xeipuuv/gojsonschema"
)

const (
	ExpectStatus = "status"
	ExpectBody   = "body"
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
	old     map[string]interface{} // For put and patch, it stores the keys used in lookup
	oldUsed map[string]interface{} // The same as the old object but with only the fields that are used in the call.
	new     map[string]interface{}
	schema  *spec.Schema
}

func (comp *Comparison) GetMapByOp(op string) map[string]interface{} {
	if op == mqswag.MethodGet {
		if comp.old == nil {
			comp.old = make(map[string]interface{})
			comp.oldUsed = make(map[string]interface{})
		}
		return comp.old
	}
	if comp.new == nil {
		comp.new = make(map[string]interface{})
	}
	return comp.new
}

func (comp *Comparison) SetForOp(op string, key string, value interface{}) *Comparison {
	var newComp *Comparison
	m := comp.GetMapByOp(op)
	if _, ok := m[key]; !ok {
		m[key] = value
	} else {
		// exist already, create a new comparison object. This only happens when we update
		// an array of objects.
		newComp := &Comparison{nil, nil, nil, comp.schema}
		m = newComp.GetMapByOp(op)
		m[key] = value
	}
	if op == mqswag.MethodGet {
		comp.oldUsed[key] = m[key]
	}
	return newComp
}

// Test represents a test object in the DSL. Extra care needs to be taken to copy the
// Test before running it, because running it would change the parameter maps.
type Test struct {
	Name       string                 `yaml:"name,omitempty"`
	Path       string                 `yaml:"path,omitempty"`
	Method     string                 `yaml:"method,omitempty"`
	Ref        string                 `yaml:"ref,omitempty"`
	Expect     map[string]interface{} `yaml:"expect,omitempty"`
	Strict     bool                   `yaml:"strict,omitempty"`
	TestParams `yaml:",inline,omitempty" json:",inline,omitempty"`

	startTime time.Time
	stopTime  time.Time

	// Map of Object name (matching definitions) to the Comparison object.
	// This tracks what objects we need to add to DB at the end of test.
	comparisons map[string]([]*Comparison)

	tag   *mqswag.MeqaTag // The tag at the top level that describes the test
	db    *mqswag.DB
	suite *TestSuite
	op    *spec.Operation
	resp  *resty.Response
	err   error

	responseError interface{}
	schemaError   error
}

func (t *Test) Init(suite *TestSuite) {
	t.suite = suite
	if suite != nil {
		t.db = suite.plan.db
	}
	if len(t.Method) != 0 {
		t.Method = strings.ToLower(t.Method)
	}
	// if BodyParams is map, after unmarshal it is map[interface{}]
	var err error
	if t.BodyParams != nil {
		t.BodyParams, err = mqutil.YamlObjToJsonObj(t.BodyParams)
		if err != nil {
			mqutil.Logger.Print(err)
		}
	}
	if len(t.Expect) > 0 && t.Expect[ExpectBody] != nil {
		t.Expect[ExpectBody], err = mqutil.YamlObjToJsonObj(t.Expect[ExpectBody])
		if err != nil {
			mqutil.Logger.Print(err)
		}
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
			op = mqswag.MethodPut
		} else {
			op = mqswag.MethodGet
		}
	}
	var comp *Comparison
	if t.comparisons[tag.Class] != nil && len(t.comparisons[tag.Class]) > 0 {
		comp = t.comparisons[tag.Class][len(t.comparisons[tag.Class])-1]
		newComp := comp.SetForOp(op, tag.Property, data)
		if newComp != nil {
			t.comparisons[tag.Class] = append(t.comparisons[tag.Class], newComp)
		}
		return
	}
	// Need to create a new compare object.
	comp = &Comparison{}
	comp.schema = (*spec.Schema)(t.db.Swagger.FindSchemaByName(tag.Class))
	comp.SetForOp(op, tag.Property, data)
	t.comparisons[tag.Class] = append(t.comparisons[tag.Class], comp)
}

func (t *Test) AddObjectComparison(tag *mqswag.MeqaTag, obj map[string]interface{}, schema *spec.Schema) {
	var class, method string
	if tag == nil {
		return
	}
	class = tag.Class
	method = tag.Operation

	// again the rule is that the child overrides the parent.
	if len(method) == 0 {
		if t.tag != nil && len(t.tag.Operation) > 0 {
			method = t.tag.Operation // At test level the tag indicates the real method
		} else {
			method = t.Method
		}
	}
	if len(class) == 0 {
		cl, s := t.db.FindMatchingSchema(obj)
		if s == nil {
			mqutil.Logger.Printf("Can't find a known schema for obj %v", obj)
			return
		}
		class = cl
	}

	if method == mqswag.MethodPost || method == mqswag.MethodPut || method == mqswag.MethodPatch {
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
		t.comparisons[class] = append(t.comparisons[class], &Comparison{nil, nil, obj, schema})
	} else {
		mqutil.Logger.Printf("unexpected: generating object %s for GET method.", class)
	}
}

func (t *Test) CompareGetResult(className string, associations map[string]map[string]interface{}, resultArray []interface{}) error {

	var dbArray []interface{}
	if len(t.comparisons[className]) > 0 {
		for _, comp := range t.comparisons[className] {
			dbArray = append(dbArray, t.db.Find(className, comp.oldUsed, associations, mqutil.InterfaceEquals, -1)...)
		}
	} else {
		dbArray = t.db.Find(className, nil, associations, mqutil.InterfaceEquals, -1)
	}
	mqutil.Logger.Printf("got %d entries from db", len(dbArray))

	// TODO optimize later. Should sort first.
	for _, entry := range resultArray {
		entryMap, _ := entry.(map[string]interface{})
		if entryMap == nil {
			// Server returned array of non-map types. Nothing for us to do. If the schema and server result doesn't
			// match we will catch that when we verify schema.
			continue
		}

		// What is returned should match our criteria.
		if len(t.comparisons[className]) > 0 {
			compFound := false
			for _, comp := range t.comparisons[className] {
				// One of the comparison should match
				if mqutil.InterfaceEquals(comp.oldUsed, entryMap) {
					compFound = true
					break
				}
			}
			if !compFound {
				b, _ := json.Marshal(entry)
				fmt.Printf("... checking GET result against client DB. Result doesn't match query. Fail\n")
				return mqutil.NewError(mqutil.ErrHttp, fmt.Sprintf("result returned doesn't match query parameters:\n%s\n",
					string(b)))
			}
		}

		if !t.Strict {
			continue
		}
		found := false
		for _, dbEntry := range dbArray {
			dbentryMap, _ := dbEntry.(map[string]interface{})
			if dbentryMap != nil && mqutil.InterfaceEquals(dbentryMap, entryMap) {
				found = true
				break
			}
		}

		if !found {
			b, _ := json.Marshal(entry)
			fmt.Printf("... checking GET result against client DB. Result not found on client. Fail\n")
			return mqutil.NewError(mqutil.ErrHttp, fmt.Sprintf("result returned is not found on client\n%s\n",
				string(b)))
		}
	}
	fmt.Printf("... checking GET result against client DB. Success\n")
	return nil
}

// ProcessOneComparison processes one comparison object.
func (t *Test) ProcessOneComparison(className string, method string, comp *Comparison,
	associations map[string]map[string]interface{}, collection map[string][]interface{}) error {

	if method == mqswag.MethodDelete {
		fmt.Printf("... deleting entry from client DB. Success\n")
		t.suite.db.Delete(className, comp.oldUsed, associations, mqutil.InterfaceEquals, -1)
		t.db.Delete(className, comp.oldUsed, associations, mqutil.InterfaceEquals, -1)
	} else if method == mqswag.MethodPost && comp.new != nil {
		fmt.Printf("... adding entry to client DB. Success\n")
		t.suite.db.Insert(className, comp.new, associations)
		return t.db.Insert(className, comp.new, associations)
	} else if (method == mqswag.MethodPatch || method == mqswag.MethodPut) && comp.new != nil {
		fmt.Printf("... updating entry in client DB. Success\n")
		t.suite.db.Update(className, comp.oldUsed, associations, mqutil.InterfaceEquals, comp.new, 1, method == mqswag.MethodPatch)
		count := t.db.Update(className, comp.oldUsed, associations, mqutil.InterfaceEquals, comp.new, 1, method == mqswag.MethodPatch)
		if count != 1 {
			mqutil.Logger.Printf("Failed to find any entry to update")
		}
	}
	return nil
}

func (t *Test) GetParam(path []string) interface{} {
	if len(path) < 2 {
		return nil
	}
	var section interface{}
	if path[0] == "pathParams" {
		section = t.PathParams
	} else if path[0] == "queryParams" {
		section = t.QueryParams
	} else if path[0] == "headerParams" {
		section = t.HeaderParams
	} else if path[0] == "formParams" {
		section = t.FormParams
	} else if path[0] == "bodyParams" {
		section = t.BodyParams
	} else if path[0] == "outputs" {
		section = t.Expect[ExpectBody]
	}

	topSection := section
	// First try the exact search. This only works if there is no
	// array on the search path.
	for _, field := range path[1:] {
		if section == nil {
			break
		}
		paramMap, ok := section.(map[string]interface{})
		if !ok {
			section = nil
			break
		}
		section = paramMap[field]
	}
	if section != nil {
		return section
	}

	// Search by iterate through all the maps. This only applies if we have only one
	// entry after the params section.
	if len(path[1:]) == 1 {
		var found interface{}
		callback := func(key string, value interface{}) error {
			if key == path[1] {
				found = value
				return mqutil.NewError(mqutil.ErrOK, "")
			}
			return nil
		}

		mqutil.IterateFieldsInInterface(topSection, callback)
		return found
	}
	return nil
}

// ProcessResult decodes the response from the server into a result array
func (t *Test) ProcessResult(resp *resty.Response) error {
	if t.err != nil {
		fmt.Printf("REST call hit the following error: %s\n", t.err.Error())
		return t.err
	}

	// useDefaultSpec := true
	t.resp = resp
	status := resp.StatusCode()
	var respSpec *spec.Response
	if t.op.Responses != nil {
		respObject, ok := t.op.Responses.StatusCodeResponses[status]
		if ok {
			respSpec = &respObject
			// useDefaultSpec = false
		} else {
			respSpec = t.op.Responses.Default
		}
	}
	if respSpec == nil {
		// Nothing specified in the swagger.json. Same as an empty spec.
		respSpec = &spec.Response{}
	}

	respBody := resp.Body()
	respSchema := (*mqswag.Schema)(respSpec.Schema)
	var resultObj interface{}
	if len(respBody) > 0 {
		d := json.NewDecoder(bytes.NewReader(respBody))
		d.UseNumber()
		d.Decode(&resultObj)
	}

	// Before returning from this function, we should set the test's expect value to that
	// of actual result. This allows us to print out a result report that is the same format
	// as the test plan file, but with the expect value that reflects the current ground truth.
	setExpect := func() {
		t.Expect = make(map[string]interface{})
		t.Expect[ExpectStatus] = status
		if resultObj != nil {
			t.Expect[ExpectBody] = resultObj
		}
	}

	if mqutil.Verbose {
		fmt.Println("Verifying REST response")
	}
	// success based on return status
	success := (status >= 200 && status < 300)
	tag := mqswag.GetMeqaTag(respSpec.Description)
	if tag != nil && tag.Flags&mqswag.FlagFail != 0 {
		success = false
	}

	testSuccess := success
	var expectedStatus interface{} = "success"
	if t.Expect != nil && t.Expect[ExpectStatus] != nil {
		expectedStatus = t.Expect[ExpectStatus]
		if expectedStatus == "fail" {
			testSuccess = !success
		} else if expectedStatusNum, ok := expectedStatus.(int); ok {
			testSuccess = (expectedStatusNum == status)
		}
	}

	greenSuccess := fmt.Sprintf("%vSuccess%v", mqutil.GREEN, mqutil.END)
	redFail := fmt.Sprintf("%vFail%v", mqutil.RED, mqutil.END)
	yellowFail := fmt.Sprintf("%vFail%v", mqutil.YELLOW, mqutil.END)
	if testSuccess {
		fmt.Printf("... expecting status: %v got status: %d. %v\n", expectedStatus, status, greenSuccess)
		if t.Expect != nil && t.Expect[ExpectBody] != nil {
			testSuccess = mqutil.InterfaceEquals(t.Expect[ExpectBody], resultObj)
			if testSuccess {
				fmt.Printf("... checking body against test's expect value. Success\n")
			} else {
				mqutil.InterfacePrint(map[string]interface{}{"... expecting body": t.Expect[ExpectBody]}, true)
				fmt.Printf("... actual response body: %s\n", respBody)
				fmt.Printf("... checking body against test's expect value. Fail\n")
				ejson, _ := json.Marshal(t.Expect[ExpectBody])
				setExpect()
				return mqutil.NewError(mqutil.ErrExpect, fmt.Sprintf(
					"=== test failed, expecting body: \n%s\ngot body:\n%s\n===", string(ejson), respBody))
			}
		}
	} else {
		t.responseError = resp
		fmt.Printf("... expecting status: %v got status: %d. %v\n", expectedStatus, status, redFail)
		setExpect()
		return mqutil.NewError(mqutil.ErrExpect, fmt.Sprintf("=== test failed, response code %d ===", status))
	}

	// Check if the response obj and respSchema match
	collection := make(map[string][]interface{})
	objMatchesSchema := false
	if resultObj != nil && respSchema != nil {
		fmt.Printf("... verifying response against openapi schema. ")
		err := respSchema.Parses("", resultObj, collection, true, t.db.Swagger)
		if err != nil {
			fmt.Printf("%v\n", yellowFail)
			objMatchesSchema = true
			specBytes, _ := json.MarshalIndent(respSpec, "", "    ")
			mqutil.Logger.Printf("server response doesn't match swagger spec: \n%s", string(specBytes))
			t.schemaError = err
			if mqutil.Verbose {
				// fmt.Printf("... openapi response schema: %s\n", string(specBytes))
				// fmt.Printf("... response body: %s\n", string(respBody))
				fmt.Println(err.Error())
			}

			// We ignore this if the response is success, and the spec we used is the default. This is a strong
			// indicator that the author didn't spec out all the success cases.
			/* Don't treat this as a hard failure for now. Too common.
			if !(useDefaultSpec && success) {
				setExpect()
				return err
			}
			*/
		} else {
			fmt.Printf("%v\n", greenSuccess)
		}
	}
	if resultObj != nil && len(collection) == 0 && t.tag != nil && len(t.tag.Class) > 0 {
		// try to resolve collection from the hint on the operation's description field.
		classSchema := t.db.GetSchema(t.tag.Class)
		if classSchema != nil {
			if classSchema.Matches(resultObj, t.db.Swagger) {
				collection[t.tag.Class] = append(collection[t.tag.Class], resultObj)
			} else {
				callback := func(value map[string]interface{}) error {
					if classSchema.Matches(value, t.db.Swagger) {
						collection[t.tag.Class] = append(collection[t.tag.Class], value)
					}
					return nil
				}
				mqutil.IterateMapsInInterface(resultObj, callback)
			}
		}
	}

	// Log some non-fatal errors.
	if respSchema != nil {
		if len(respBody) > 0 {
			if resultObj == nil && !respSchema.Type.Contains(gojsonschema.TYPE_STRING) {
				specBytes, _ := json.MarshalIndent(respSpec, "", "    ")
				mqutil.Logger.Printf("server response doesn't match swagger spec: \n%s", string(specBytes))
			}
		} else {
			// If schema is an array, then not having a body is OK
			if !respSchema.Type.Contains(gojsonschema.TYPE_ARRAY) {
				mqutil.Logger.Printf("swagger.spec expects a non-empty response, but response body is actually empty")
			}
		}
	}
	if expectedStatus != "success" {
		setExpect()
		return nil
	}

	// Sometimes the server will return more data than requested. For instance, the server may generate
	// a uuid that the client didn't send. So for post and puts, we first go through the collection.
	// The assumption is that if the server returned an object of matching type, we should use that
	// instead of what the client thinks.
	method := t.Method
	if t.tag != nil && len(t.tag.Operation) > 0 {
		method = t.tag.Operation
	}

	// For posts, it's possible that the server has replaced certain fields (such as uuid). We should just
	// use the server's result.
	if method == mqswag.MethodPost {
		var propertyCollection map[string][]interface{}
		if objMatchesSchema {
			propertyCollection = make(map[string][]interface{})
			respSchema.Parses("", resultObj, propertyCollection, false, t.db.Swagger)
		}

		for className, compList := range t.comparisons {
			if len(compList) > 0 && compList[0].new != nil {
				classList := collection[className]
				if len(classList) > 0 {
					// replace what we posted with what the server returned
					var newcompList []*Comparison
					for _, entry := range classList {
						c := Comparison{nil, nil, entry.(map[string]interface{}), nil}
						newcompList = append(newcompList, &c)
					}
					collection[className] = nil
					t.comparisons[className] = newcompList
				} else if len(compList) == 1 {
					// When posting a single item, and the server returned fields that belong to the object,
					// replace that field with what the server returned.
					for k, v := range propertyCollection {
						keyAr := strings.Split(k, ".")
						if len(keyAr) == 2 && keyAr[0] == className && len(v) == 1 {
							compList[0].new[keyAr[1]] = v[0]
						}
					}
				}
			}
		}
	}

	// Associations are only for the objects that has one for each class and has an old object.
	associations := make(map[string]map[string]interface{})
	for className, compArray := range t.comparisons {
		if len(compArray) == 1 && compArray[0].oldUsed != nil {
			associations[className] = compArray[0].oldUsed
		}
	}

	if method == mqswag.MethodGet {
		// For gets, we process based on the result collection's class.
		for className, resultArray := range collection {
			err := t.CompareGetResult(className, associations, resultArray)
			if err != nil {
				setExpect()
				return err
			}
		}
	} else {
		for className, compArray := range t.comparisons {
			for _, c := range compArray {
				err := t.ProcessOneComparison(className, method, c, associations, collection)
				if err != nil {
					setExpect()
					return err
				}
			}
		}
	}

	if !t.Strict {
		// Add everything from the collection to the in-mem DB
		for className, classList := range collection {
			for _, entry := range classList {
				t.db.Insert(className, entry, associations)
			}
		}
	}

	setExpect()
	return nil
}

// SetRequestParameters sets the parameters. Returns the new request path.
func (t *Test) SetRequestParameters(req *resty.Request) string {
	files := make(map[string]string)
	for _, p := range t.op.Parameters {
		if p.Type == "file" && t.FormParams[p.Name] != nil {
			// for swagger 2 file type can only be in formData
			if fname, ok := t.FormParams[p.Name].(string); ok {
				files[p.Name] = fname
				delete(t.FormParams, p.Name)
			}
		}
	}
	if len(files) > 0 {
		req.SetFiles(files)
	}
	if len(t.FormParams) > 0 {
		req.SetFormData(mqutil.MapInterfaceToMapString(t.FormParams))
		mqutil.InterfacePrint(map[string]interface{}{"formParams": t.FormParams}, mqutil.Verbose)
	}
	for k, v := range files {
		t.FormParams[k] = v
	}

	if len(t.QueryParams) > 0 {
		req.SetQueryParams(mqutil.MapInterfaceToMapString(t.QueryParams))
		mqutil.InterfacePrint(map[string]interface{}{"queryParams": t.QueryParams}, mqutil.Verbose)
	}
	if t.BodyParams != nil {
		req.SetBody(t.BodyParams)
		mqutil.InterfacePrint(map[string]interface{}{"bodyParams": t.BodyParams}, mqutil.Verbose)
	}
	if len(t.HeaderParams) > 0 {
		req.SetHeaders(mqutil.MapInterfaceToMapString(t.HeaderParams))
		mqutil.InterfacePrint(map[string]interface{}{"headerParams": t.HeaderParams}, mqutil.Verbose)
	}
	path := t.Path
	if len(t.PathParams) > 0 {
		PathParamsStr := mqutil.MapInterfaceToMapString(t.PathParams)
		for k, v := range PathParamsStr {
			path = strings.Replace(path, "{"+k+"}", v, -1)
		}
		mqutil.InterfacePrint(map[string]interface{}{"pathParams": t.PathParams}, mqutil.Verbose)
	}
	return path
}

func (t *Test) CopyParent(parentTest *Test) {
	if parentTest != nil {
		t.Strict = parentTest.Strict
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
func (t *Test) Run(tc *TestSuite) error {

	mqutil.Logger.Print("\n--- " + t.Name)
	fmt.Printf("\nRunning test case: %s\n", t.Name)
	err := t.ResolveParameters(tc)
	if err != nil {
		fmt.Printf("... Fail\n... %s\n", err.Error())
		return err
	}

	req := resty.R()
	if len(tc.ApiToken) > 0 {
		req.SetAuthToken(tc.ApiToken)
	} else if len(tc.Username) > 0 {
		req.SetBasicAuth(tc.Username, tc.Password)
	}

	path := GetBaseURL(t.db.Swagger) + t.SetRequestParameters(req)
	var resp *resty.Response

	t.startTime = time.Now()
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
	t.stopTime = time.Now()
	fmt.Printf("... call completed: %f seconds\n", t.stopTime.Sub(t.startTime).Seconds())

	if err != nil {
		t.err = mqutil.NewError(mqutil.ErrHttp, err.Error())
	} else {
		mqutil.Logger.Print(resp.Status())
		mqutil.Logger.Println(string(resp.Body()))
	}
	err = t.ProcessResult(resp)
	return err
}

func StringParamsResolveWithHistory(str string, h *TestHistory) interface{} {
	begin := strings.Index(str, "{{")
	end := strings.Index(str, "}}")
	if end > begin {
		ar := strings.Split(strings.Trim(str[begin+2:end], " "), ".")
		if len(ar) < 3 {
			mqutil.Logger.Printf("invalid parameter: {{%s}}, the format is {{testName.paramSection.paramName}}, e.g. {{test1.output.id}}",
				str[begin+2:end])
			return nil
		}
		t := h.GetTest(ar[0])
		if t != nil {
			return t.GetParam(ar[1:])
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
func (t *Test) ResolveParameters(tc *TestSuite) error {
	pathItem := t.db.Swagger.Paths.Paths[t.Path]
	t.op = GetOperationByMethod(&pathItem, t.Method)
	if t.op == nil {
		return mqutil.NewError(mqutil.ErrNotFound, fmt.Sprintf("Path %s not found in swagger file", t.Path))
	}
	fmt.Printf("... resolving parameters.\n")

	// There can be parameters at the path level. We merge these with the operation parameters.
	t.op.Parameters = ParamsAdd(t.op.Parameters, pathItem.Parameters)

	t.tag = mqswag.GetMeqaTag(t.op.Description)

	var paramsMap map[string]interface{}
	var globalParamsMap map[string]interface{}
	var err error
	var genParam interface{}
	for _, params := range t.op.Parameters {
		fmt.Printf("        %s (in %s): ", params.Name, params.In)
		if params.In == "body" {
			var bodyMap map[string]interface{}
			bodyIsMap := false
			if t.BodyParams != nil {
				bodyMap, bodyIsMap = t.BodyParams.(map[string]interface{})
			}
			if t.BodyParams != nil && !bodyIsMap {
				// Body is not map, we use it directly.
				paramTag, schema := t.db.Swagger.GetSchemaRootType((*mqswag.Schema)(params.Schema), mqswag.GetMeqaTag(params.Description))
				if schema != nil && paramTag != nil {
					objarray, _ := t.BodyParams.([]interface{})
					for _, obj := range objarray {
						objMap, ok := obj.(map[string]interface{})
						if ok {
							t.AddObjectComparison(paramTag, objMap, (*spec.Schema)(schema))
						}
					}
				}
				fmt.Print("provided\n")
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
			_, inLocal := paramsMap[params.Name]
			_, inGlobal := globalParamsMap[params.Name]
			if !inLocal && inGlobal {
				paramsMap[params.Name] = globalParamsMap[params.Name]
			}
			if _, ok := paramsMap[params.Name]; ok {
				t.AddBasicComparison(mqswag.GetMeqaTag(params.Description), &params, paramsMap[params.Name])
				fmt.Print("provided\n")
				continue
			}
			genParam, err = t.GenerateParameter(&params, t.db)
			paramsMap[params.Name] = genParam
		}
		if err != nil {
			return err
		}
	}
	paramMaps := []*map[string]interface{}{&t.PathParams, &t.QueryParams, &t.HeaderParams, &t.FormParams}
	for _, m := range paramMaps {
		removeNulls(m)
	}
	if t.BodyParams != nil {
		bodyMap := t.BodyParams.(map[string]interface{})
		removeNulls(&bodyMap)
		t.BodyParams = bodyMap
	}
	return nil
}

func removeNulls(inputMap *map[string]interface{}) {
	filteredMap := make(map[string]interface{})
	for k, v := range *inputMap {
		if v != nil {
			filteredMap[k] = v
		}
	}
	*inputMap = filteredMap
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
		return t.GenerateSchema("", tag, paramSpec.Schema, db, 3)
	}
	if len(paramSpec.Enum) != 0 {
		fmt.Print("enum\n")
		return generateEnum(paramSpec.Enum)
	}
	if len(paramSpec.Type) == 0 {
		return nil, mqutil.NewError(mqutil.ErrInvalid, "Parameter doesn't have type")
	}

	// construct a full schema from simple ones
	schema := (*spec.Schema)(mqswag.CreateSchemaFromSimple(&paramSpec.SimpleSchema, &paramSpec.CommonValidations))
	if paramSpec.Type == gojsonschema.TYPE_OBJECT {
		return t.generateObject("", tag, schema, db, 3)
	}
	if paramSpec.Type == gojsonschema.TYPE_ARRAY {
		return t.generateArray("", tag, schema, db, 3)
	}

	return t.generateByType(schema, paramSpec.Name, tag, paramSpec, true)
}

// Two ways to get to generateByType
// 1) directly called from GenerateParameter, now we know the type is a parameter, and we want to add to comparison
// 2) called at bottom level, here we know the object will be added to comparison and not the type primitives.
func (t *Test) generateByType(s *spec.Schema, prefix string, parentTag *mqswag.MeqaTag, paramSpec *spec.Parameter, print bool) (interface{}, error) {
	tag := mqswag.GetMeqaTag(s.Description)
	if tag == nil {
		tag = parentTag
	}
	if paramSpec != nil {
		if tag != nil && len(tag.Property) > 0 {
			// Try to get one from the comparison objects.
			for _, c := range t.comparisons[tag.Class] {
				if c.old != nil {
					c.oldUsed[tag.Property] = c.old[tag.Property]
					if print {
						fmt.Printf("found %s.%s\n", tag.Class, tag.Property)
					}
					return c.old[tag.Property], nil
				}
			}
			// Get one from in-mem db and populate the comparison structure.
			ar := t.suite.db.Find(tag.Class, nil, nil, mqswag.MatchAlways, 5)
			if len(ar) == 0 {
				ar = t.db.Find(tag.Class, nil, nil, mqswag.MatchAlways, 5)
			}
			if len(ar) > 0 {
				obj := ar[rand.Intn(len(ar))].(map[string]interface{})
				comp := &Comparison{obj, make(map[string]interface{}), nil, (*spec.Schema)(t.db.GetSchema(tag.Class))}
				comp.oldUsed[tag.Property] = comp.old[tag.Property]
				t.comparisons[tag.Class] = append(t.comparisons[tag.Class], comp)
				if print {
					fmt.Printf("found %s.%s\n", tag.Class, tag.Property)
				}
				return obj[tag.Property], nil
			}
		}
	}

	if len(s.Type) != 0 {
		if print {
			fmt.Print("random\n")
		}
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
			return nil, errors.New("can not automatically upload a file, parameter of file type must be manually set\n")
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
	if s.Format == "uuid" {
		u, err := uuid.NewV4()
		return u.String(), err
	}
	if s.Format == "email" {
		s.Pattern = "^\\w+@[a-zA-Z_]+?\\.[a-zA-Z]{2,3}$"
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

	if len(s.Format) == 0 || s.Format == "password" || s.Format == "email" {
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
		maxf := 1000000.0
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

func (t *Test) generateArray(name string, parentTag *mqswag.MeqaTag, schema *spec.Schema, db *mqswag.DB, level int) (interface{}, error) {
	var numItems int
	if schema.MaxItems != nil || schema.MinItems != nil {
		var maxItems int = 10
		if schema.MaxItems != nil {
			maxItems = int(*schema.MaxItems)
			if maxItems <= 0 {
				maxItems = 1
			}
		}
		var minItems int
		if schema.MinItems != nil {
			minItems = int(*schema.MinItems)
			if minItems <= 0 {
				minItems = 1
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
	if numItems <= 0 {
		numItems = 1
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

	generateOneEntry := func() error {
		entry, err := t.GenerateSchema(name, tag, itemSchema, db, level)
		if err != nil {
			return err
		}
		if entry == nil {
			return nil
		}
		if hash != nil && hash[entry] != nil {
			return nil
		}
		ar = append(ar, entry)
		if hash != nil {
			hash[entry] = 1
		}
		return nil
	}

	// we only print one entry
	err := generateOneEntry()
	if err != nil {
		return nil, err
	}
	level = 0 // this will supress prints
	for i := 0; i < numItems; i++ {
		err = generateOneEntry()
		if err != nil {
			return nil, err
		}
	}
	return ar, nil
}

func (t *Test) generateObject(name string, parentTag *mqswag.MeqaTag, schema *spec.Schema, db *mqswag.DB, level int) (interface{}, error) {
	obj := make(map[string]interface{})
	var spaces string
	var nextLevel int
	if level > 0 {
		spaces = strings.Repeat("    ", level)
		nextLevel = level + 1
	}
	if level != 0 {
		fmt.Println("")
	}
	for k, v := range schema.Properties {
		if level != 0 {
			fmt.Printf("%s%s . ", spaces, k)
		}
		o, err := t.GenerateSchema(k+"_", nil, &v, db, nextLevel)
		if err != nil {
			return nil, err
		}
		obj[k] = o
	}

	tag := mqswag.GetMeqaTag(schema.Description)
	if tag == nil {
		tag = parentTag
	}

	if tag != nil {
		t.AddObjectComparison(tag, obj, schema)
	}
	return obj, nil
}

// The parentTag passed in is what the higher level thinks this schema object should be.
func (t *Test) GenerateSchema(name string, parentTag *mqswag.MeqaTag, schema *spec.Schema, db *mqswag.DB, level int) (interface{}, error) {
	swagger := db.Swagger

	// The tag that's closest to the object takes priority, much like child class can override parent class.
	tag := mqswag.GetMeqaTag(schema.Description)
	if tag == nil {
		tag = parentTag
	}

	// Deal with refs.
	referenceName, referredSchema, err := swagger.GetReferredSchema((*mqswag.Schema)(schema))
	if err != nil {
		return nil, err
	}
	if referredSchema != nil {
		if len(name) > 0 {
			// This the the field of an object. Instead of generating a new object, we try to get one
			// from the DB. If we can't find one, we put in null.
			found := t.suite.db.Find(referenceName, nil, nil, mqswag.MatchAlways, 1)
			if len(found) == 0 {
				found = t.db.Find(referenceName, nil, nil, mqswag.MatchAlways, 1)
			}
			if len(found) > 0 {
				if level != 0 {
					fmt.Printf("found %s\n", referenceName)
				}
				return found[0], nil
			}
			if level != 0 {
				fmt.Printf("null\n")
			}
			return nil, nil
		}
		return t.GenerateSchema(name, &mqswag.MeqaTag{referenceName, "", "", 0}, (*spec.Schema)(referredSchema), db, level)
	}

	if len(schema.Enum) != 0 {
		if level != 0 {
			fmt.Print("enum\n")
		}
		return generateEnum(schema.Enum)
	}

	if len(schema.AllOf) > 0 {
		combined := make(map[string]interface{})
		discriminator := ""
		for _, s := range schema.AllOf {
			m, err := t.GenerateSchema(name, nil, &s, db, level)
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
			if len(s.Discriminator) > 0 {
				discriminator = s.Discriminator
			} else {
				// This is more common, the discriminator is in a common object referred from AllOf
				_, rs, _ := swagger.GetReferredSchema((*mqswag.Schema)(&s))
				if rs != nil && len(rs.Discriminator) > 0 {
					discriminator = rs.Discriminator
				}
			}
		}
		if len(discriminator) > 0 && len(tag.Class) > 0 {
			combined[discriminator] = tag.Class
		}
		// Add combined to the comparison under tag.
		t.AddObjectComparison(tag, combined, schema)
		return combined, nil
	}

	if len(schema.Type) == 0 {
		// return nil, mqutil.NewError(mqutil.ErrInvalid, "Parameter doesn't have type")
		return t.generateObject(name, tag, schema, db, level)
	}
	if schema.Type[0] == gojsonschema.TYPE_OBJECT {
		return t.generateObject(name, tag, schema, db, level)
	}
	if schema.Type[0] == gojsonschema.TYPE_ARRAY {
		return t.generateArray(name, tag, schema, db, level)
	}

	return t.generateByType(schema, name, tag, nil, level != 0)
}

func generateEnum(e []interface{}) (interface{}, error) {
	return e[rand.Intn(len(e))], nil
}
