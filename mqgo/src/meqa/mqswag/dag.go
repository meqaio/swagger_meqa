package mqswag

import (
	"fmt"
	"meqa/mqutil"
	"sort"
)

const (
	DAGDepth = 1000
)

// The traversal order is from this node to children. The children depend on the parent.
// The children's weight would be bigger than the parent.
type DAGNode struct {
	Name     string
	Weight   int
	Data     interface{}
	Children NodeList

	dag *DAG
}

// AdjustWeight changes the children's weight to be at least this node's weight + 1
func (node *DAGNode) AdjustChildrenWeight() error {
	for _, c := range node.Children {
		if c.Weight <= node.Weight {
			err := node.dag.AdjustNodeWeight(c, node.Weight+1)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (node *DAGNode) CheckChildrenWeight() bool {
	for _, c := range node.Children {
		if c.Weight <= node.Weight {
			return false
		}
		mqutil.Logger.Printf("child: %s", c.Name)
	}
	return true
}

func (node *DAGNode) AddChild(child *DAGNode) error {
	// Checks like these aren't necessary once our code works correctly. For now it makes catching bugs easier.
	for _, c := range node.Children {
		if c.Name == child.Name {
			// Objects have unique name, therefore child has unique name.
			return nil
		}
	}
	node.Children = append(node.Children, child)
	if child.Weight <= node.Weight {
		return node.AdjustChildrenWeight()
	}
	return nil
}

type NodeList []*DAGNode

func (n NodeList) Len() int {
	return len(n)
}

func (n NodeList) Swap(i, j int) {
	n[i], n[j] = n[j], n[i]
}

func (n NodeList) Less(i, j int) bool {
	return n[i].Name < n[j].Name
}

// We expect a single thread on the server would handle the DAG creation and traversing. So no mutex for now.
type DAG struct {
	NameMap    map[string]*DAGNode // DAGNode name to node mapping.
	WeightList [DAGDepth]NodeList  // List ordered by DAGNodes' weights. Max of 1000 levels in DAG depth.
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
	if node == nil || node.Weight < 0 || node.Weight >= DAGDepth || node.dag != dag {
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
			node.AdjustChildrenWeight()
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

// sort the dag by name
func (dag *DAG) Sort() {
	for w := 0; w < DAGDepth; w++ {
		sort.Sort(dag.WeightList[w])
	}

	sortChildren := func(previous *DAGNode, current *DAGNode) error {
		sort.Sort(current.Children)
		return nil
	}
	dag.IterateByWeight(sortChildren)
}

func (dag *DAG) CheckWeight() {
	checkChildren := func(previous *DAGNode, current *DAGNode) error {
		mqutil.Logger.Printf("name: %s weight: %d", current.Name, current.Weight)
		ok := current.CheckChildrenWeight()
		if !ok {
			panic("bad weight detected")
		}
		return nil
	}
	dag.IterateByWeight(checkChildren)
}

func NewDAG() *DAG {
	d := &DAG{}
	d.NameMap = make(map[string]*DAGNode)
	return d
}
