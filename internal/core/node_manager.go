package core

import (
	"github.com/heyvito/eswim/internal/logutil"
	"github.com/heyvito/eswim/internal/proto"
	"go.uber.org/zap"
	"math/rand"
	"slices"
	"sync"
)

// NodeManager holds a list of nodes and handles all state transitions between
// them. It also provides nodes to be used as indirect probes, and retains the
// node order for ping operations during the protocol period.
type NodeManager interface {
	NodeList

	// Bootstrap initializes both the internal node list and the ping list from
	// a list provided by another node. See also nodeList.Bootstrap.
	Bootstrap(period uint16, list []proto.Node)

	// Add inserts a node to the member and ping lists. See also nodeList.Add.
	Add(period uint16, node *proto.Node)

	// MarkFaulty removes a node from both the member and node lists. See also
	// nodeList.MarkFaulty.
	MarkFaulty(hash proto.NodeHash)

	// NextPing returns the next node to be pinged as part of the protocol period.
	// Returns nil in case no node is currently in the members list.
	NextPing() *proto.Node

	// UpdateOrCreate updates or creates the provided node, considering its
	// provided status.
	UpdateOrCreate(period uint16, node *proto.Node)

	// WrapIncarnation forcefully updates a given node to contain a new
	// incarnation identifier provided by the `node` parameter.
	WrapIncarnation(period uint16, node *proto.Node)

	// All returns a copy of the list of all nodes known by this manager that
	// matches the provided filter.
	All(filter NodeType) []proto.Node
}

// NewNodeManager returns a new nodeManager
func NewNodeManager(logger *zap.Logger, selfAddr proto.IP) NodeManager {
	return &nodeManager{
		selfAddr:    selfAddr,
		nodeList:    NewNodeList(logger, selfAddr).(*nodeList),
		nodesToPing: nil,
		logger:      logger.With(zap.String("facility", "node_manager")),
	}
}

type nodeManager struct {
	mu sync.Mutex
	*nodeList
	nodesToPing []proto.NodeHash
	selfAddr    proto.IP
	logger      *zap.Logger
}

func (n *nodeManager) All(filter NodeType) []proto.Node {
	var listCopy []proto.Node
	n.Range(filter, func(n *proto.Node) bool {
		listCopy = append(listCopy, *n)
		return true
	})
	return listCopy
}

// unsafePreparePingList prepares a ping list based on the current view of the
// cluster. mu must be held in write mode.
func (n *nodeManager) unsafePreparePingList() {
	n.nodesToPing = n.nodesToPing[:0] // just in case
	n.Range(NodeTypeStable|NodeTypeSuspect, func(node *proto.Node) bool {
		n.nodesToPing = append(n.nodesToPing, node.Hash())
		return true
	})

	rand.Shuffle(len(n.nodesToPing), func(i, j int) {
		n.nodesToPing[i], n.nodesToPing[j] = n.nodesToPing[j], n.nodesToPing[i]
	})
	n.logger.Debug("Ping list", logutil.StringerArr("nodesToPing", n.nodesToPing))
}

func (n *nodeManager) Bootstrap(period uint16, list []proto.Node) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.nodeList.Bootstrap(period, list)
	n.unsafePreparePingList()
}

func (n *nodeManager) WrapIncarnation(period uint16, node *proto.Node) {
	n.mu.Lock()
	defer n.mu.Unlock()
	hash := node.Hash()
	localNode := n.NodeByHash(hash)

	if localNode == nil {
		n.Add(period, node)
		return
	}
	n.nodeList.MarkStable(hash)
	localNode.Incarnation = node.Incarnation
}

func (n *nodeManager) UpdateOrCreate(period uint16, node *proto.Node) {
	hash := node.Hash()
	localNode := n.NodeByHash(hash)

	if localNode == nil {
		// DO NOT hold n.mu at this point, as it will cause a nasty deadlock.
		n.Add(period, node)
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if localNode.Incarnation > node.Incarnation {
		return
	}
	localNode.Incarnation = node.Incarnation

	if node.Suspect {
		n.nodeList.MarkSuspect(period, true, hash)
	} else {
		n.nodeList.MarkStable(hash)
	}
}

func (n *nodeManager) MarkFaulty(hash proto.NodeHash) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.nodeList.MarkFaulty(hash)

	idxToRemove := slices.Index(n.nodesToPing, hash)
	if idxToRemove == -1 {
		return
	}
	n.nodesToPing = slices.Delete(n.nodesToPing, idxToRemove, idxToRemove+1)
}

func (n *nodeManager) NextPing() *proto.Node {
	n.mu.Lock()
	defer n.mu.Unlock()

	pingLen := len(n.nodesToPing)
	if pingLen == 0 {
		return nil
	}

	toPing := n.nodesToPing[0]
	n.nodesToPing = n.nodesToPing[1:]
	n.logger.Debug(
		"Next ping is returning",
		zap.String("hash", toPing.String()),
		logutil.StringerArr("nodesToPing", n.nodesToPing),
	)

	if len(n.nodesToPing) == 0 {
		n.unsafePreparePingList()
	}

	return n.NodeByHash(toPing)
}
