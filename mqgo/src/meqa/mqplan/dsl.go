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

	"meqa/mqswag"
	"meqa/mqutil"
)

func MapInterfaceToMapString(src map[string]interface{}) map[string]string {
	dst := make(map[string]string)
	for k, v := range src {
		dst[k] = fmt.Sprint(v)
	}
	return dst
}

// Test represents a test object in the DSL
type Test struct {
	Name         string
	Path         string
	Method       string
	Ref          string
	QueryParams  map[string]interface{}
	BodyParams   map[string]interface{}
	FormParams   map[string]interface{}
	PathParams   map[string]interface{}
	HeaderParams map[string]interface{}
}

// Run runs the test. It only returns error when there is an internal error.
// Test case failures are not counted.
func (t *Test) Run(swagger *mqswag.Swagger, db mqswag.DB, plan *TestPlan) error {
	if len(t.Ref) != 0 {
		return plan.Run(t.Ref, swagger, db)
	}
	err := t.ResolveParameters(swagger, db, plan)
	if err != nil {
		return err
	}

	// TODO add check for http/https (operation schemes) and pointers
	switch t.Method {
	case resty.MethodGet:
		// TODO add other types of params
		resp, err := resty.R().SetQueryParams(MapInterfaceToMapString(t.QueryParams)).
			Get(swagger.BasePath + "/" + t.Path)
		// TODO properly process resp. Check against the current DB to see if they match
		mqutil.Logger.Print(resp)

		return err
	default:
		str := fmt.Sprintf("Unknow method in test %s: %v", t.Name, t.Method)
		return errors.New(str)
	}
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
			paramsMap = t.PathParams
		case "query":
			paramsMap = t.QueryParams
		case "header":
			paramsMap = t.HeaderParams
		case "body":
			paramsMap = t.BodyParams
		case "form":
			paramsMap = t.FormParams
		}
		// We don't override the existing parameters
		if _, ok := paramsMap[params.Name]; ok {
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
			if len(t.Method) != 0 {
				t.Method = strings.ToUpper(t.Method)
			}
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
func (plan *TestPlan) Run(name string, swagger *mqswag.Swagger, db mqswag.DB) (err error) {
	tc, ok := plan.CaseMap[name]
	if !ok || len(tc) == 0 {
		str := fmt.Sprintf("The following test case is not found: %s", name)
		mqutil.Logger.Println(str)
		return errors.New(str)
	}

	for _, test := range tc {
		err = test.Run(swagger, db, plan)
		if err != nil {
			return err
		}

	}
	return nil
}

// The current global TestPlan
var Current TestPlan

func init() {
	rand.Seed(int64(time.Now().Second()))
}
