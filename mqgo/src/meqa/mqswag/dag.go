package mqswag

import (
	"fmt"
	"meqa/mqutil"
)

const (
	DAGDepth = 1000
)

type DAGNode struct {
	Name     string
	Weight   int
	Data     interface{}
	Children []*DAGNode

	dag *DAG
}

// AdjustWeight changes the node's weight to that of max(children) + 1
func (node *DAGNode) AdjustWeight() error {
	maxChildrenWeight := 0
	for _, n := range node.Children {
		if n.Weight > maxChildrenWeight {
			maxChildrenWeight = n.Weight
		}
	}
	if maxChildrenWeight+1 != node.Weight {
		return node.dag.AdjustNodeWeight(node, maxChildrenWeight+1)
	}
	return nil
}

func (node *DAGNode) AddChild(child *DAGNode) error {
	// Checks like these aren't necessary once our code works correctly. For now it makes catching bugs easier.
	for _, c := range node.Children {
		if c.Name == child.Name {
			return mqutil.NewError(mqutil.ErrInvalid, fmt.Sprintf("adding node to an existing name: %v", child))
		}
	}
	node.Children = append(node.Children, child)
	if child.Weight >= node.Weight {
		return node.AdjustWeight()
	}
	return nil
}

// We expect a single thread on the server would handle the DAG creation and traversing. So no mutex for now.
type DAG struct {
	NameMap    map[string]*DAGNode  // DAGNode name to node mapping.
	WeightList [DAGDepth][]*DAGNode // List ordered by DAGNodes' weights. Max of 1000 levels in DAG depth.
}

func (dag *DAG) Init() {
	dag.NameMap = make(map[string]*DAGNode)
}

func (dag *DAG) NewNode(name string, data interface{}) (*DAGNode, error) {
	node := &DAGNode{name, 0, data, nil, dag}
	err := dag.AddNode(node)
	if err != nil {
		return nil, err
	}
	return node, nil
}

func (dag *DAG) AddNode(node *DAGNode) error {
	if node == nil || node.Weight < 0 || node.Weight >= DAGDepth || node.dag != nil {
		return mqutil.NewError(mqutil.ErrInvalid, fmt.Sprintf("adding an invalid DAG Node: %v", node))
	}
	if dag.NameMap[node.Name] != nil {
		return mqutil.NewError(mqutil.ErrInvalid, fmt.Sprintf("adding node to an existing name: %v", node))
	}

	node.dag = dag
	dag.NameMap[node.Name] = node
	dag.WeightList[node.Weight] = append(dag.WeightList[node.Weight], node)
	return nil
}

func (dag *DAG) AdjustNodeWeight(node *DAGNode, newWeight int) error {
	if dag.NameMap[node.Name] != node {
		return mqutil.NewError(mqutil.ErrInvalid, fmt.Sprintf("changing the weight of a node that's not found in dag: %v", node))
	}
	l := dag.WeightList[node.Weight]
	for i, n := range l {
		if n.Name == node.Name {
			l[i] = l[0]
			dag.WeightList[node.Weight] = l[1:]
			node.Weight = newWeight
			dag.WeightList[node.Weight] = append(dag.WeightList[node.Weight], node)
			return nil
		}
	}
	return mqutil.NewError(mqutil.ErrInvalid, fmt.Sprintf("changing the weight of a node that's not found in dag: %v", node))
}

type DAGIterFunc func(previous *DAGNode, current *DAGNode) error

func (dag *DAG) IterateWeight(weight int, f DAGIterFunc) error {
	if weight >= DAGDepth {
		return mqutil.NewError(mqutil.ErrInvalid, fmt.Sprintf("invalid weight to iterate", weight))
	}
	l := dag.WeightList[weight]
	for _, n := range l {
		err := f(nil, n)
		if err != nil {
			return err
		}
	}
	return nil
}

func (dag *DAG) IterateByWeight(f DAGIterFunc) error {
	for w := 0; w < DAGDepth; w++ {
		err := dag.IterateWeight(w, f)
		if err != nil {
			return err
		}
	}
	return nil
}

func NewDAG() *DAG {
	d := &DAG{}
	d.NameMap = make(map[string]*DAGNode)
	return d
}
