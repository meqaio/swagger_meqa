package mqplan

import (
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"meqa/mqswag"
	"meqa/mqutil"
)

type TestCase struct {
	Tests []*Test
	Name  string
}

// Represents all the test cases in the DSL.
type TestPlan struct {
	CaseMap  map[string](*TestCase)
	CaseList [](*TestCase)
	db       *mqswag.DB
}

// Add a new TestCase, returns whether the Case is successfully added.
func (plan *TestPlan) Add(name string, testCase *TestCase) error {
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
	var caseMap map[string]([]*Test)
	err := yaml.Unmarshal([]byte(data), &caseMap)
	if err != nil {
		mqutil.Logger.Printf("The following is not a valud TestCase:\n%s", data)
		return err
	}
	for caseName, testList := range caseMap {
		for _, t := range testList {
			t.Init(plan.db)
		}
		testCase := TestCase{testList, caseName}
		err = plan.Add(caseName, &testCase)
		if err != nil {
			return err
		}
	}
	return nil
}

func (plan *TestPlan) InitFromFile(path string, db *mqswag.DB) error {
	plan.db = db
	plan.CaseMap = make(map[string]*TestCase)
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
	if !ok || len(tc.Tests) == 0 {
		str := fmt.Sprintf("The following test case is not found: %s", name)
		mqutil.Logger.Println(str)
		return nil, errors.New(str)
	}

	var output []map[string]interface{}
	for _, test := range tc.Tests {
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
