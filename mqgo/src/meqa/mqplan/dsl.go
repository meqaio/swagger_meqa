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
	"reflect"
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

// MapIsCompatible checks if the first map has every key in the second.
func MapIsCompatible(big map[string]interface{}, small map[string]interface{}) bool {
	for k, _ := range small {
		if _, ok := big[k]; !ok {
			return false
		}
	}
	return true
}

// MapCombine combines two map together. If there is any overlap the dst will be overwritten.
func MapCombine(dst map[string]interface{}, src map[string]interface{}) map[string]interface{} {
	if len(dst) == 0 {
		return src
	}
	if len(src) == 0 {
		return dst
	}
	for k, v := range src {
		dst[k] = v
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
	Name         string
	Path         string
	Method       string
	Ref          string
	QueryParams  map[string]interface{} `yaml:"queryParams"`
	BodyParams   interface{}            `yaml:"bodyParams"`
	FormParams   map[string]interface{} `yaml:"formParams"`
	PathParams   map[string]interface{} `yaml:"pathParams"`
	HeaderParams map[string]interface{} `yaml:"headerParams"`

	db *mqswag.DB
}

func (t *Test) Init(db *mqswag.DB) {
	t.db = db
	if len(t.Method) != 0 {
		t.Method = strings.ToUpper(t.Method)
	}
	// if BodyParams is map, after unmarshal it is map[interface{}]
	if bodyMap, ok := t.BodyParams.(map[interface{}]interface{}); ok {
		newMap := make(map[string]interface{})
		for k, v := range bodyMap {
			newMap[fmt.Sprint(k)] = v
		}
		t.BodyParams = newMap
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
	if len(t.QueryParams) > 0 {
		req.SetQueryParams(MapInterfaceToMapString(t.QueryParams))
	}
	if t.BodyParams != nil {
		req.SetBody(t.BodyParams)
	}
	if len(t.HeaderParams) > 0 {
		req.SetHeaders(MapInterfaceToMapString(t.HeaderParams))
	}
	if len(t.FormParams) > 0 {
		req.SetFormData(MapInterfaceToMapString(t.FormParams))
	}
	if len(t.PathParams) > 0 {
		PathParamsStr := MapInterfaceToMapString(t.PathParams)
		for k, v := range PathParamsStr {
			strings.Replace(t.Path, "{"+k+"}", v, -1)
		}
	}
}

// Run runs the test. Returns the test result.
func (t *Test) Run(plan *TestPlan, parentTest *Test) ([]map[string]interface{}, error) {

	if parentTest != nil {
		t.QueryParams = MapCombine(t.QueryParams, parentTest.QueryParams)
		t.PathParams = MapCombine(t.PathParams, parentTest.PathParams)
		t.HeaderParams = MapCombine(t.HeaderParams, parentTest.HeaderParams)
		t.FormParams = MapCombine(t.FormParams, parentTest.FormParams)

		if parentTest.BodyParams != nil {
			if t.BodyParams == nil {
				t.BodyParams = parentTest.BodyParams
			} else {
				// replace with parent only if the types are the same
				if parentBodyMap, ok := parentTest.BodyParams.(map[string]interface{}); ok {
					if bodyMap, ok := t.BodyParams.(map[string]interface{}); ok {
						t.BodyParams = MapCombine(bodyMap, parentBodyMap)
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

	err := t.ResolveParameters(plan)
	if err != nil {
		return nil, err
	}

	// We do this after resolving all parameters. The next level will inherit
	// what the parent (this test) decides.
	if len(t.Ref) != 0 {
		return plan.Run(t.Ref, t)
	}

	req := resty.R()
	t.SetRequestParameters(req)
	path := GetBaseURL(t.db.Swagger) + t.Path
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
func (t *Test) ResolveParameters(plan *TestPlan) error {
	pathItem := t.db.Swagger.Paths.Paths[t.Path]
	op := getOperationByMethod(&pathItem, t.Method)
	if op == nil {
		return mqutil.NewError(mqutil.ErrNotFound, fmt.Sprintf("Path %s not found in swagger file", t.Path))
	}

	var paramsMap map[string]interface{}
	var err error
	var genParam interface{}
	for _, params := range op.Parameters {
		if params.In == "body" {
			if t.BodyParams != nil {
				// There is only one body parameter. No need to check name. In fact, we don't
				// even store the name in the DSL.
				continue
			}
			genParam, err = GenerateParameter(&params, t.db)
			t.BodyParams = genParam
		} else {
			switch params.In {
			case "path":
				if t.PathParams == nil {
					t.PathParams = make(map[string]interface{})
				}
				paramsMap = t.PathParams
			case "query":
				if t.QueryParams == nil {
					t.QueryParams = make(map[string]interface{})
				}
				paramsMap = t.QueryParams
			case "header":
				if t.HeaderParams == nil {
					t.HeaderParams = make(map[string]interface{})
				}
				paramsMap = t.HeaderParams
			case "formData":
				if t.FormParams == nil {
					t.FormParams = make(map[string]interface{})
				}
				paramsMap = t.FormParams
			}

			// If there is a parameter passed in, just use it. Otherwise generate one.
			if _, ok := paramsMap[params.Name]; ok {
				continue
			}
			genParam, err = GenerateParameter(&params, t.db)
			paramsMap[params.Name] = genParam
		}
		if err != nil {
			return err
		}
	}
	return nil
}

type TestCase []*Test

// Represents all the test cases in the DSL.
type TestPlan struct {
	CaseMap  map[string](TestCase)
	CaseList [](TestCase)
	db       *mqswag.DB
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
			t.Init(plan.db)
		}
		err = plan.Add(testName, testCase)
		if err != nil {
			return err
		}
	}
	return nil
}

func (plan *TestPlan) InitFromFile(path string, db *mqswag.DB) error {
	plan.db = db
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
func (plan *TestPlan) Run(name string, parentTest *Test) ([]map[string]interface{}, error) {
	tc, ok := plan.CaseMap[name]
	if !ok || len(tc) == 0 {
		str := fmt.Sprintf("The following test case is not found: %s", name)
		mqutil.Logger.Println(str)
		return nil, errors.New(str)
	}

	var output []map[string]interface{}
	for _, test := range tc {
		result, err := test.Run(plan, parentTest)
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
