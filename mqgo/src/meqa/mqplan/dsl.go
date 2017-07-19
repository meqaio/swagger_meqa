package mqplan

import (
	"io/ioutil"
	"meqa/mqutil"
	"strings"

	"errors"
	"fmt"

	"gopkg.in/yaml.v2"
)

// Test represents a test object in the DSL
type Test struct {
	Name       string
	Path       string
	Method     string
	Ref        string
	Parameters map[string]interface{}
}

type TestCase struct {
	Tests []*Test
}

// Represents all the test cases in the DSL.
type TestPlan struct {
	CaseMap  map[string](*TestCase)
	CaseList [](*TestCase)
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
	var caseMap map[string](*TestCase)
	err := yaml.Unmarshal([]byte(data), &caseMap)
	if err != nil {
		mqutil.Logger.Printf("The following is not a valud TestCase:\n%s", data)
		return err
	}
	for k, v := range caseMap {
		err = plan.Add(k, v)
		if err != nil {
			return err
		}
	}
	return nil
}

func (plan *TestPlan) InitFromFile(path string) error {
	plan.CaseMap = make(map[string](*TestCase))
	plan.CaseList = nil

	data, err := ioutil.ReadFile(path)
	if err != nil {
		mqutil.Logger.Printf("Can't open the following file: %s", path)
		mqutil.Logger.Println(err.Error())
		return err
	}
	chunks := strings.Split(string(data), "---")
	for _, chunk := range chunks {
		err := plan.AddFromString(chunk)
		if err != nil {
			return err
		}
	}
	return nil
}

// Run a named TestCase in the test plan.
func (plan *TestPlan) Run(name string) error {
	panic("not implemented")
	return nil
}

// The current global TestPlan
var Current TestPlan
