package core

import (
	"github.com/heyvito/eswim/internal/proto"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"testing"
)

// NodeHandler abstracts any implementation that's capable of handling nodes,
// such as NodeList and NodeManager. This is intended for testing-purposes only.
type nodeHandler interface {
	Add(period uint16, node *proto.Node)
	MarkStable(hash proto.NodeHash)
	MarkSuspect(period uint16, remote bool, hash proto.NodeHash)
	MarkFaulty(hash proto.NodeHash)
	suspectList() map[proto.NodeHash]*Suspect
}

var selfIP proto.IP = proto.IP{AddressKind: proto.AddressIPv4}

func makeNodeList() *nodeList { return NewNodeList(zap.NewNop(), selfIP).(*nodeList) }

func addNode(t *testing.T, period uint16, target nodeHandler, nType NodeType) *proto.Node {
	n := &proto.Node{
		Suspect:     nType.Suspect(),
		Incarnation: 0,
		Address:     makeIP(t),
	}
	target.Add(period, n)

	if nType.Suspect() && !nType.Remote() {
		target.suspectList()[n.Hash()].FromRemote = false
	}
	return n
}

func TestNodeList_Bootstrap(t *testing.T) {
	n := makeNodeList()

	n1 := proto.Node{
		Suspect:     false,
		Incarnation: 0,
		Address:     makeIP(t),
	}
	n2 := proto.Node{
		Suspect:     true,
		Incarnation: 1,
		Address:     makeIP(t),
	}

	n.Bootstrap(1, []proto.Node{n1, n2})

	assert.Len(t, n.nodeByType, 2)
	assert.Len(t, n.suspects, 1)
	assert.Len(t, n.stable, 1)
	assert.Equal(t, uint16(1), n.suspects[n2.Hash()].SuspectAtPeriod)
	assert.True(t, n.suspects[n2.Hash()].FromRemote)
}

func TestNodeList_Add(t *testing.T) {
	n := makeNodeList()

	n1 := &proto.Node{
		Suspect:     false,
		Incarnation: 0,
		Address:     makeIP(t),
	}
	n2 := &proto.Node{
		Suspect:     true,
		Incarnation: 1,
		Address:     makeIP(t),
	}

	n.Add(1, n1)
	n.Add(1, n2)

	assert.Len(t, n.nodeByType, 2)
	assert.Len(t, n.suspects, 1)
	assert.Len(t, n.stable, 1)
	assert.Equal(t, uint16(1), n.suspects[n2.Hash()].SuspectAtPeriod)
	assert.True(t, n.suspects[n2.Hash()].FromRemote)
}

func TestNodeList_Range(t *testing.T) {
	t.Run("Common iteration", func(t *testing.T) {
		n := makeNodeList()

		cases := map[NodeType]*proto.Node{
			NodeTypeStable:                   nil,
			NodeTypeSuspect:                  nil,
			NodeTypeSuspect | NodeTypeRemote: nil,
		}
		for k := range cases {
			cases[k] = addNode(t, 1, n, k)
		}

		for k, v := range cases {
			if k.Suspect() && !k.Remote() {
				continue // Suspect will also include remote
			}
			var rangeResults []*proto.Node
			n.Range(k, func(n *proto.Node) bool {
				rangeResults = append(rangeResults, n)
				return true
			})
			assert.Len(t, rangeResults, 1, "Incorrect length for type %d", k)
			assert.Equal(t, v, rangeResults[0])
		}

		var rangeResults []*proto.Node
		n.Range(NodeTypeSuspect, func(n *proto.Node) bool {
			rangeResults = append(rangeResults, n)
			return true
		})
		assert.Len(t, rangeResults, 2)

		rangeResults = rangeResults[:0]
		n.Range(0, func(n *proto.Node) bool {
			rangeResults = append(rangeResults, n)
			return true
		})
		assert.Len(t, rangeResults, 3)
	})

	t.Run("Iteration stop", func(t *testing.T) {
		n := makeNodeList()
		s := addNode(t, 1, n, NodeTypeStable)
		addNode(t, 1, n, NodeTypeSuspect)

		var rangeResults []*proto.Node
		n.Range(0, func(n *proto.Node) bool {
			rangeResults = append(rangeResults, n)
			return false
		})
		assert.Len(t, rangeResults, 1)
		assert.Equal(t, s, rangeResults[0])
	})
}

func TestNodeList_MarkStable(t *testing.T) {
	n := makeNodeList()
	node := addNode(t, 1, n, NodeTypeSuspect|NodeTypeRemote)
	assert.Len(t, n.suspects, 1)
	n.MarkStable(node.Hash())
	assert.Len(t, n.stable, 1)
	assert.NotNil(t, n.NodeByHash(node.Hash())) // make sure it is still indexed
}

func TestNodeList_MarkSuspect(t *testing.T) {
	n := makeNodeList()

	node := addNode(t, 1, n, NodeTypeStable)
	assert.Len(t, n.stable, 1)
	assert.Len(t, n.suspects, 0)
	n.MarkSuspect(2, false, node.Hash())
	assert.Len(t, n.suspects, 1)
	assert.Len(t, n.stable, 0)
	assert.NotNil(t, n.NodeByHash(node.Hash())) // make sure it is still indexed
	suspect := n.suspects[node.Hash()]
	assert.Equal(t, uint16(2), suspect.SuspectAtPeriod)
	assert.Equal(t, false, suspect.FromRemote)
}

func TestNodeList_MarkFaulty(t *testing.T) {
	n := makeNodeList()

	node := addNode(t, 1, n, NodeTypeStable)
	assert.Len(t, n.stable, 1)
	n.MarkFaulty(node.Hash())
	assert.Len(t, n.suspects, 0)
	assert.Len(t, n.stable, 0)
}

func TestNodeList_ExpiredSuspects(t *testing.T) {
	n := makeNodeList()

	node := addNode(t, 1, n, NodeTypeStable).Hash()
	addNode(t, 1, n, NodeTypeSuspect|NodeTypeRemote).Hash()

	n.MarkSuspect(2, false, node)

	nodes := n.ExpiredSuspects(3, 1)
	assert.Len(t, nodes, 1)
	assert.Len(t, n.suspects, 1)
	assert.Equal(t, node, nodes[0].Hash())

}

func TestNodeList_Delete(t *testing.T) {
	n := makeNodeList()

	node1 := addNode(t, 1, n, NodeTypeStable)
	node2 := addNode(t, 1, n, NodeTypeSuspect)
	assert.Len(t, n.stable, 1)
	assert.Len(t, n.suspects, 1)
	n.Delete(node1.Hash())
	n.Delete(node2.Hash())
	assert.Len(t, n.suspects, 0)
	assert.Len(t, n.stable, 0)
}

func TestNodeList_IndirectPingCandidates(t *testing.T) {
	n := makeNodeList()

	node1 := addNode(t, 1, n, NodeTypeStable)
	node2 := addNode(t, 1, n, NodeTypeSuspect)
	node3 := addNode(t, 1, n, NodeTypeStable)

	l := n.IndirectPingCandidates(2, node3.Hash())
	assert.Len(t, l, 2)
	assert.ElementsMatch(t, l, []*proto.Node{node1, node2})

	l = n.IndirectPingCandidates(3, node3.Hash())
	assert.Len(t, l, 2)

	l = n.IndirectPingCandidates(1, node3.Hash())
	assert.Len(t, l, 1)
}
