package core

import (
	"github.com/heyvito/eswim/internal/proto"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"testing"
)

func makeNodeManager() *nodeManager {
	return NewNodeManager(zap.NewNop(), selfIP).(*nodeManager)
}

func TestNodeManager_Add(t *testing.T) {
	nm := makeNodeManager()
	assert.Empty(t, nm.nodesToPing)
	addNode(t, 1, nm, NodeTypeStable)
	nm.unsafePreparePingList()
	assert.NotEmpty(t, nm.nodesToPing)
}

func TestNodeManager_Bootstrap(t *testing.T) {
	nm := makeNodeManager()
	assert.Empty(t, nm.nodesToPing)
	var nodes []proto.NodeHash
	for i := 0; i < 10; i++ {
		nodes = append(nodes, addNode(t, 1, nm, NodeTypeStable).Hash())
	}
	nm.unsafePreparePingList()
	assert.Len(t, nm.nodesToPing, 10)
	assert.ElementsMatch(t, nodes, nm.nodesToPing)

	// Just to make sure insertAt is doing its job
	hash := addNode(t, 1, nm, NodeTypeStable).Hash()
	nm.unsafePreparePingList()
	assert.Contains(t, nm.nodesToPing, hash)
}

func TestNodeManager_MarkFaulty(t *testing.T) {
	nm := makeNodeManager()
	assert.Empty(t, nm.nodesToPing)
	hash := addNode(t, 1, nm, NodeTypeStable).Hash()
	nm.unsafePreparePingList()
	assert.NotEmpty(t, nm.nodesToPing)
	nm.MarkFaulty(hash)
	nm.unsafePreparePingList()
	assert.Empty(t, nm.nodesToPing)
}

func TestNodeManager_NextPing(t *testing.T) {
	nm := makeNodeManager()
	addNode(t, 1, nm, NodeTypeStable)
	addNode(t, 1, nm, NodeTypeStable)
	nm.unsafePreparePingList()
	assert.Len(t, nm.nodesToPing, 2)
	nm.NextPing()
	assert.Len(t, nm.nodesToPing, 1)
	nm.NextPing()
	assert.Len(t, nm.nodesToPing, 2)
}
