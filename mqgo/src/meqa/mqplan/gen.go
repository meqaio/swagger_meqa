package mqplan

import (
	"fmt"
	"meqa/mqswag"
	"sort"
	"strings"

	"github.com/go-openapi/spec"
)

// Given a path name, retrieve the last entry that is not a path param.
func GetLastPathElement(name string) string {
	nameArray := strings.Split(name, "/")
	for i := len(nameArray) - 1; i >= 0; i-- {
		if len(nameArray[i]) > 0 && nameArray[i][0] != '{' {
			return nameArray[i]
		}
	}
	return ""
}

func CreateTestFromOp(opNode *mqswag.DAGNode, testId int) *Test {
	op := opNode.Data.((*spec.Operation))
	t := &Test{}
	t.Path = opNode.GetName()
	t.Method = opNode.GetMethod()
	opId := op.ID
	if len(opId) == 0 {
		opId = GetLastPathElement(t.Path)
	}
	t.Name = fmt.Sprintf("%s_%s_%d", t.Method, opId, testId)

	return t
}

func OperationIsDelete(node *mqswag.DAGNode) bool {
	op, ok := node.Data.(*spec.Operation)
	if ok && op != nil {
		tag := mqswag.GetMeqaTag(op.Description)
		if (tag != nil && tag.Operation == mqswag.MethodDelete) || (tag == nil && node.GetMethod() == mqswag.MethodDelete) {
			return true
		}
	}
	return false
}

// GenerateTestsForObject for the obj that we traversed to from create. Add the test cases
// generated to plan.
func GenerateTestsForObject(create *mqswag.DAGNode, obj *mqswag.DAGNode, plan *TestPlan) error {
	if obj.GetType() != mqswag.TypeDef {
		return nil
	}
	if create.GetType() != mqswag.TypeOp {
		return nil
	}
	createPath := create.GetName()
	objName := obj.GetName()

	// A loop where we go through all the child operations
	testId := 1
	testCase := CreateTestCase(fmt.Sprintf("%s -- %s -- all", createPath, objName), nil, plan)
	testCase.Tests = append(testCase.Tests, CreateTestFromOp(create, testId))
	for _, child := range obj.Children {
		if child.GetType() != mqswag.TypeOp {
			continue
		}
		testId++
		testCase.Tests = append(testCase.Tests, CreateTestFromOp(child, testId))
		if OperationIsDelete(child) {
			testId++
			testCase.Tests = append(testCase.Tests, CreateTestFromOp(create, testId))
		}
	}
	if len(testCase.Tests) > 0 {
		plan.Add(testCase)
	}

	// a loop where we pick random operations and pair it with the create operation.
	// This would generate a few objects.
	/* disable random stuff during development
	testId = 0
	testCase = &TestCase{nil, fmt.Sprintf("%s -- %s -- random", createPath, objName)}
	for i := 0; i < 2*len(obj.Children); i++ {
		j := rand.Intn(len(obj.Children))
		child := obj.Children[j]
		if child.GetType() != mqswag.TypeOp {
			mqutil.Logger.Printf("unexpected: (%s) has a child (%s) that's not an operation", obj.Name, child.Name)
			continue
		}
		testId++
		testCase.Tests = append(testCase.Tests, CreateTestFromOp(create, testId))
		testId++
		testCase.Tests = append(testCase.Tests, CreateTestFromOp(child, testId))
	}
	if len(testCase.Tests) > 0 {
		plan.Add(testCase)
	}
	*/

	return nil
}

func GenerateTestPlan(swagger *mqswag.Swagger, dag *mqswag.DAG) (*TestPlan, error) {
	testPlan := &TestPlan{}
	testPlan.Init(swagger, nil)

	genFunc := func(previous *mqswag.DAGNode, current *mqswag.DAGNode) error {
		if current.GetType() != mqswag.TypeOp {
			return nil
		}

		// Exercise the function by itself.
		testCase := CreateTestCase(current.GetName()+" "+current.GetMethod(), nil, testPlan)
		testCase.Tests = append(testCase.Tests, CreateTestFromOp(current, 1))
		testPlan.Add(testCase)

		// When iterating by weight previous is always nil.
		for _, c := range current.Children {
			err := GenerateTestsForObject(current, c, testPlan)
			if err != nil {
				return err
			}
		}

		return nil
	}
	err := dag.IterateByWeight(genFunc)
	if err != nil {
		return nil, err
	}
	return testPlan, nil
}

// All the operations have the same path. We generate one test case, with the
// tests of ascending weight and priority among the operations
func GeneratePathTestCase(operations mqswag.NodeList, plan *TestPlan) {
	if len(operations) == 0 {
		return
	}

	pathName := operations[0].GetName()
	sort.Sort(operations)
	testId := 0
	testCase := CreateTestCase(fmt.Sprintf("%s", pathName), nil, plan)
	for _, o := range operations {
		testId++
		testCase.Tests = append(testCase.Tests, CreateTestFromOp(o, testId))
	}
	if len(testCase.Tests) > 0 {
		plan.Add(testCase)
	}
}

type PathWeight struct {
	path   string
	weight int
}

type PathWeightList []PathWeight

func (n PathWeightList) Len() int {
	return len(n)
}

func (n PathWeightList) Swap(i, j int) {
	n[i], n[j] = n[j], n[i]
}

func (n PathWeightList) Less(i, j int) bool {
	return n[i].weight < n[j].weight || (n[i].weight == n[j].weight && n[i].path < n[j].path)
}

// Go through all the paths in swagger, and generate the tests for all the operations under
// the path.
func GeneratePathTestPlan(swagger *mqswag.Swagger, dag *mqswag.DAG) (*TestPlan, error) {
	testPlan := &TestPlan{}
	testPlan.Init(swagger, nil)

	pathMap := make(map[string]mqswag.NodeList)
	pathWeight := make(map[string]int)

	addFunc := func(previous *mqswag.DAGNode, current *mqswag.DAGNode) error {
		if current.GetType() != mqswag.TypeOp {
			return nil
		}
		name := current.GetName()

		// if the last path element is a {..} path param we remove it. Also remove the ending "/"
		// because it has no effect.
		nameArray := strings.Split(name, "/")
		if len(nameArray) > 0 && len(nameArray[len(nameArray)-1]) == 0 {
			nameArray = nameArray[:len(nameArray)-1]
		}
		if len(nameArray) > 0 {
			if last := nameArray[len(nameArray)-1]; len(last) > 0 && last[0] == '{' && last[len(last)-1] == '}' {
				nameArray = nameArray[:len(nameArray)-1]
			}
		}
		name = strings.Join(nameArray, "/")

		pathMap[name] = append(pathMap[name], current)

		currentWeight := current.Weight*mqswag.DAGDepth + current.Priority
		if pathWeight[name] <= currentWeight {
			pathWeight[name] = currentWeight
		}

		return nil
	}

	dag.IterateByWeight(addFunc)

	var pathWeightList PathWeightList
	// Sort the path by weight
	for k, v := range pathWeight {
		p := PathWeight{k, v}
		pathWeightList = append(pathWeightList, p)
	}
	sort.Sort(pathWeightList)

	for _, p := range pathWeightList {
		GeneratePathTestCase(pathMap[p.path], testPlan)
	}
	return testPlan, nil
}
