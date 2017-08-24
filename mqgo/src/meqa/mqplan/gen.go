package mqplan

import (
	"fmt"
	"meqa/mqswag"

	"github.com/go-openapi/spec"
)

func CreateTestFromOp(opNode *mqswag.DAGNode, testId int) *Test {
	op := opNode.Data.((*spec.Operation))
	t := &Test{}
	t.Name = fmt.Sprintf("%s_%d", op.ID, testId)
	t.Path = mqswag.GetName(opNode.Name)
	t.Method = mqswag.GetMethod(opNode.Name)

	return t
}

func OperationIsDelete(node *mqswag.DAGNode) bool {
	op, ok := node.Data.(*spec.Operation)
	if ok && op != nil {
		tag := mqswag.GetMeqaTag(op.Description)
		if (tag != nil && tag.Operation == mqswag.MethodDelete) || (tag == nil && mqswag.GetMethod(node.Name) == mqswag.MethodDelete) {
			return true
		}
	}
	return false
}

// GenerateTestsForObject for the obj that we traversed to from create. Add the test cases
// generated to plan.
func GenerateTestsForObject(create *mqswag.DAGNode, obj *mqswag.DAGNode, plan *TestPlan) error {
	if mqswag.GetType(obj.Name) != mqswag.TypeDef {
		return nil
	}
	if mqswag.GetType(create.Name) != mqswag.TypeOp {
		return nil
	}
	createPath := mqswag.GetName(create.Name)
	objName := mqswag.GetName(obj.Name)

	// A loop where we go through all the child operations
	testId := 1
	testCase := CreateTestCase(fmt.Sprintf("%s -- %s -- all", createPath, objName), nil, plan)
	testCase.Tests = append(testCase.Tests, CreateTestFromOp(create, testId))
	for _, child := range obj.Children {
		if mqswag.GetType(child.Name) != mqswag.TypeOp {
			continue
		}
		testId++
		testCase.Tests = append(testCase.Tests, CreateTestFromOp(child, testId))
		if OperationIsDelete(child) {
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
		if mqswag.GetType(child.Name) != mqswag.TypeOp {
			mqutil.Logger.Printf("unexpected: (%s) has a child (%s) that's not an operation", obj.Name, child.Name)
			continue
		}
		testId++
		testCase.Tests = append(testCase.Tests, CreateTestFromOp(create, testId))
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
		if mqswag.GetType(current.Name) != mqswag.TypeOp {
			return nil
		}

		// Exercise the function by itself.
		testCase := CreateTestCase(mqswag.GetName(current.Name)+" "+mqswag.GetMethod(current.Name), nil, testPlan)
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
