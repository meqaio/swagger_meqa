package mqplan

import (
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"strings"
	"time"

	"gopkg.in/resty.v0"
	"gopkg.in/yaml.v2"

	"encoding/json"
	"meqa/mqswag"
	"meqa/mqutil"
)

// MapInterfaceToMapString converts the params map (all primitive types with exception of array)
// before passing to resty.
func MapInterfaceToMapString(src map[string]interface{}) map[string]string {
	dst := make(map[string]string)
	for k, v := range src {
		if ar, ok := v.([]interface{}); ok {
			str := ""
			for _, entry := range ar {
				str += fmt.Sprintf("%v,", entry)
			}
			str = strings.TrimRight(str, ",")
			dst[k] = str
		} else {
			dst[k] = fmt.Sprint(v)
		}
	}
	return dst
}

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

// Test represents a test object in the DSL
type Test struct {
	Name       string
	Path       string
	Method     string
	Ref        string
	Parameters map[string]interface{}

	queryParams  map[string]interface{}
	bodyParams   map[string]interface{}
	formParams   map[string]interface{}
	pathParams   map[string]interface{}
	headerParams map[string]interface{}
}

func (t *Test) Init() {
	if len(t.Method) != 0 {
		t.Method = strings.ToUpper(t.Method)
	}
	if t.Parameters == nil {
		t.Parameters = make(map[string]interface{})
	}
}

// DecodeResult decodes the response from the server into a result array
func (t *Test) DecodeResult(resp *resty.Response) ([]map[string]interface{}, error) {
	var resultArray []map[string]interface{}
	err := json.Unmarshal(resp.Body(), &resultArray)
	if err == nil {
		return resultArray, nil
	}
	var resultMap map[string]interface{}
	err = json.Unmarshal(resp.Body(), &resultMap)
	if err == nil {
		return []map[string]interface{}{resultMap}, nil
	}
	return nil, err
}

// SetRequestParameters sets the parameters
func (t *Test) SetRequestParameters(req *resty.Request) {
	if len(t.queryParams) > 0 {
		req.SetQueryParams(MapInterfaceToMapString(t.queryParams))
	}
	if len(t.bodyParams) > 0 {
		req.SetBody(t.bodyParams)
	}
	if len(t.headerParams) > 0 {
		req.SetHeaders(MapInterfaceToMapString(t.headerParams))
	}
	if len(t.formParams) > 0 {
		req.SetFormData(MapInterfaceToMapString(t.formParams))
	}
	if len(t.pathParams) > 0 {
		pathParamsStr := MapInterfaceToMapString(t.pathParams)
		for k, v := range pathParamsStr {
			strings.Replace(t.Path, "{"+k+"}", v, -1)
		}
	}
}

// Run runs the test. Returns the test result.
func (t *Test) Run(swagger *mqswag.Swagger, db mqswag.DB, plan *TestPlan, params map[string]interface{}) ([]map[string]interface{}, error) {
	if len(t.Ref) != 0 {
		return plan.Run(t.Ref, swagger, db, params)
	}

	// Add parameters passed in to t's existing parameters. What is passed in from outside
	// always takes higher priority
	for k, v := range params {
		t.Parameters[k] = v
	}
	err := t.ResolveParameters(swagger, db, plan)
	if err != nil {
		return nil, err
	}

	req := resty.R()
	t.SetRequestParameters(req)
	path := GetBaseURL(swagger) + t.Path
	var resp *resty.Response

	switch t.Method {
	case resty.MethodGet:
		resp, err = req.Get(path)
	case resty.MethodPost:
		resp, err = req.Post(path)
	case resty.MethodPut:
		resp, err = req.Put(path)
	case resty.MethodDelete:
		resp, err = req.Delete(path)
	case resty.MethodPatch:
		resp, err = req.Patch(path)
	case resty.MethodHead:
		resp, err = req.Head(path)
	case resty.MethodOptions:
		resp, err = req.Options(path)
	default:
		return nil, mqutil.NewError(mqutil.ErrInvalid, fmt.Sprintf("Unknown method in test %s: %v", t.Name, t.Method))
	}
	if err != nil {
		return nil, mqutil.NewError(mqutil.ErrHttp, err.Error())
	}
	// TODO properly process resp. Check against the current DB to see if they match
	return t.DecodeResult(resp)
}

// ResolveParameters fullfills the parameters for the specified request using the in-mem DB.
// The resolved parameters will be added to test.Parameters map.
func (t *Test) ResolveParameters(swagger *mqswag.Swagger, db mqswag.DB, plan *TestPlan) error {
	pathItem := swagger.Paths.Paths[t.Path]
	op := getOperationByMethod(&pathItem, t.Method)
	if op == nil {
		return mqutil.NewError(mqutil.ErrNotFound, fmt.Sprintf("Path %s not found in swagger file", t.Path))
	}

	var paramsMap map[string]interface{}
	for _, params := range op.Parameters {
		switch params.In {
		case "path":
			if t.pathParams == nil {
				t.pathParams = make(map[string]interface{})
			}
			paramsMap = t.pathParams
		case "query":
			if t.queryParams == nil {
				t.queryParams = make(map[string]interface{})
			}
			paramsMap = t.queryParams
		case "header":
			if t.headerParams == nil {
				t.headerParams = make(map[string]interface{})
			}
			paramsMap = t.headerParams
		case "body":
			if t.bodyParams == nil {
				t.bodyParams = make(map[string]interface{})
			}
			paramsMap = t.bodyParams
		case "form":
			if t.formParams == nil {
				t.formParams = make(map[string]interface{})
			}
			paramsMap = t.formParams
		}
		// If there is a parameter passed in, just use it.
		if _, ok := t.Parameters[params.Name]; ok {
			paramsMap[params.Name] = t.Parameters[params.Name]
			continue
		}
		p, err := GenerateParameter(&params, swagger, db)
		if err != nil {
			return err
		}
		paramsMap[params.Name] = p
		return nil
	}
	return nil
}

type TestCase []*Test

// Represents all the test cases in the DSL.
type TestPlan struct {
	CaseMap  map[string](TestCase)
	CaseList [](TestCase)
}

// Add a new TestCase, returns whether the Case is successfully added.
func (plan *TestPlan) Add(name string, testCase TestCase) error {
	if _, exist := plan.CaseMap[name]; exist {
		str := fmt.Sprintf("Duplicate name %s found in test plan", name)
		mqutil.Logger.Println(str)
		return errors.New(str)
	}
	plan.CaseMap[name] = testCase
	plan.CaseList = append(plan.CaseList, testCase)
	return nil
}

func (plan *TestPlan) AddFromString(data string) error {
	var caseMap map[string]TestCase
	err := yaml.Unmarshal([]byte(data), &caseMap)
	if err != nil {
		mqutil.Logger.Printf("The following is not a valud TestCase:\n%s", data)
		return err
	}
	for testName, testCase := range caseMap {
		for _, t := range testCase {
			t.Init()
		}
		err = plan.Add(testName, testCase)
		if err != nil {
			return err
		}
	}
	return nil
}

func (plan *TestPlan) InitFromFile(path string) error {
	plan.CaseMap = make(map[string]TestCase)
	plan.CaseList = nil

	data, err := ioutil.ReadFile(path)
	if err != nil {
		mqutil.Logger.Printf("Can't open the following file: %s", path)
		mqutil.Logger.Println(err.Error())
		return err
	}
	chunks := strings.Split(string(data), "---")
	for _, chunk := range chunks {
		plan.AddFromString(chunk)
	}
	return nil
}

// Run a named TestCase in the test plan.
func (plan *TestPlan) Run(name string, swagger *mqswag.Swagger, db mqswag.DB, params map[string]interface{}) ([]map[string]interface{}, error) {
	tc, ok := plan.CaseMap[name]
	if !ok || len(tc) == 0 {
		str := fmt.Sprintf("The following test case is not found: %s", name)
		mqutil.Logger.Println(str)
		return nil, errors.New(str)
	}

	var output []map[string]interface{}
	for _, test := range tc {
		result, err := test.Run(swagger, db, plan, params)
		if err != nil {
			return nil, err
		}
		output = append(output, result...)
	}
	return output, nil
}

// The current global TestPlan
var Current TestPlan

func init() {
	rand.Seed(int64(time.Now().Second()))
}
