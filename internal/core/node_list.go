package core

import (
	"fmt"
	"github.com/heyvito/eswim/internal/proto"
	"go.uber.org/zap"
	"math"
	"sync"
)

// stringer can be installed from golang.org/x/tools/cmd/stringer@latest
//go:generate stringer -type=NodeType

type NodeType uint8

func (n NodeType) Is(o NodeType) bool { return n&o == o }
func (n NodeType) Suspect() bool      { return n.Is(NodeTypeSuspect) }
func (n NodeType) Stable() bool       { return n.Is(NodeTypeStable) }
func (n NodeType) Remote() bool       { return n.Is(NodeTypeRemote) }

const (
	NodeTypeStable NodeType = 1 << iota
	NodeTypeSuspect
	NodeTypeRemote

	NodeTypeInvalid NodeType = 0
	NodeTypeAll              = NodeTypeStable | NodeTypeSuspect
)

// Suspect represents a suspect node in the cluster
type Suspect struct {
	// Node contains information about the node being suspected of being
	// at fault.
	Node *proto.Node

	// SuspectAtPeriod represents the protocol period number when this node was
	// assumed as unstable by the current node.
	SuspectAtPeriod uint16

	// FromRemote indicates whether this suspect was assumed as suspect by the
	// current node (false), of it was assumed as suspect through the gossip
	// sub-protocol. When set, this suspect node must be handled normally by
	// the suspect timeout check, but once it expires, instead of disseminating
	// a faulty message, the current node will only silently mark it as faulty,
	// as the responsibility of disseminating the proto.EventFaulty pertains
	// to the node that announced it as suspect. This prevents a stale suspect
	// from staying in the node list in case the responsible for announcing it
	// goes down before announcing it as faulty.
	FromRemote bool
}

func (s Suspect) String() string {
	return fmt.Sprintf("{Suspect Node: %s, SuspectAtPeriod: %d, FromRemote: %t}",
		s.Node, s.SuspectAtPeriod, s.FromRemote)
}

type NodeList interface {
	// Add stores a given node in case it is not a member of the current list
	Add(currentPeriod uint16, node *proto.Node)

	// Range ranges over nodes present in this list. Filter can be used to
	// limit which kind of node will be yielded. NodeType is a bitfield, meaning
	// it can be bitwise ORed in order to compose filters. The parameter fn is a
	// function that will be called for each node matching the provided filter;
	// its return value determines whether iteration should continue.
	// Do not attempt use other methods during ranges, as it will result in a
	// deadlock.
	Range(filter NodeType, fn func(n *proto.Node) bool)

	// MarkStable marks a previously added node with proto.NodeHash `hash` as
	// stable, in case it has any other state.
	MarkStable(hash proto.NodeHash)

	// MarkSuspect moves a node identified by a given `hash` to the suspect list
	// under the current protocol `period`. `remote` indicates whether the node
	// is being marked as suspect due to a report from a remote peer. If the
	// current instance has deemed the hash as suspicious by itself, this value
	// must be set to `false`.
	MarkSuspect(period uint16, remote bool, hash proto.NodeHash)

	// MarkFaulty marks a node with given `hash` as Faulty (by removing it from
	// the members list).
	MarkFaulty(hash proto.NodeHash)

	// ExpiredSuspects returns a list of nodes that have status NodeTypeSuspect
	// for at least `expirationRounds` relative to the `currentPeriod`. This
	// method does not return nodes marked as suspect by other nodes; those are
	// automatically expunged as Faulty.
	ExpiredSuspects(currentPeriod, expirationRounds uint16) []*proto.Node

	// Delete removes a node identified by a given proto.NodeHash from the
	// members list, ignoring its current state.
	Delete(hash proto.NodeHash)

	// Bootstrap accepts an initial list of nodes to be added to the current
	// member list.
	Bootstrap(period uint16, list []proto.Node)

	// NodeByHash returns a known node identified by the provided proto.NodeHash
	NodeByHash(hash proto.NodeHash) *proto.Node

	// NodeByIP work like NodeByHash, but searching for a node with a given IP
	// address.
	NodeByIP(ip proto.IP) *proto.Node

	// IndirectPingCandidates returns at most k random nodes from the list of
	// members to be used as indirect ping delegates.
	IndirectPingCandidates(k int, except proto.NodeHash) []*proto.Node
}

// NewNodeList returns a new nodeList
func NewNodeList(logger *zap.Logger, selfAddr proto.IP) NodeList {
	return &nodeList{
		selfAddr:   selfAddr,
		nodeByType: map[proto.NodeHash]NodeType{},
		stable:     map[proto.NodeHash]*proto.Node{},
		suspects:   map[proto.NodeHash]*Suspect{},
		mu:         sync.RWMutex{},
		logger:     logger.Named("node_list"),
	}
}

// nodeList retains a list of nodes known by the current instance
type nodeList struct {
	nodeByType map[proto.NodeHash]NodeType
	stable     map[proto.NodeHash]*proto.Node
	suspects   map[proto.NodeHash]*Suspect
	mu         sync.RWMutex
	selfAddr   proto.IP
	logger     *zap.Logger
}

// suspectList returns a reference to the current suspect list without any kind
// of locking mechanism. This must be only used by testing facilities.
func (n *nodeList) suspectList() map[proto.NodeHash]*Suspect { return n.suspects }

// unsafeAdd attempts to add a given node found or updated in a given period.
// This method will not update data that's already present in the member list.
// mu must be held in write mode.
func (n *nodeList) unsafeAdd(period uint16, node *proto.Node) {
	if n.selfAddr.Equal(&node.Address) {
		return
	}

	hash := node.Hash()

	if node, ok := n.nodeByType[hash]; ok {
		n.logger.Debug("Skipping known node",
			zap.String("node", node.String()),
			zap.String("hash", hash.String()),
			zap.Bool("suspect", node.Suspect()),
			zap.Bool("stable", node.Stable()),
		)

		return
	}

	if !node.Suspect {
		n.nodeByType[hash] = NodeTypeStable
		n.stable[hash] = node
		return
	}

	n.nodeByType[hash] = NodeTypeSuspect
	n.suspects[hash] = &Suspect{
		Node:            node,
		SuspectAtPeriod: period,
		FromRemote:      true,
	}
}

func (n *nodeList) Add(currentPeriod uint16, node *proto.Node) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.unsafeAdd(currentPeriod, node)
}

func (n *nodeList) Range(filter NodeType, fn func(n *proto.Node) bool) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if filter == NodeTypeInvalid {
		filter = NodeTypeAll
	}

	wantsRemote := filter.Remote()
	if filter.Stable() {
		for _, v := range n.stable {
			if !fn(v) {
				return
			}
		}
	}
	if filter.Suspect() {
		for _, v := range n.suspects {
			if wantsRemote && !v.FromRemote {
				continue
			}
			if !fn(v.Node) {
				return
			}
		}
	}
}

// unsafeTakeAndDelete obtains a proto.Node with a given proto.NodeHash from
// a list of type `source`. Returns a pointer to the node, and a bool indicating
// whether the node has been found (and deleted). Source ignores“ values other
// than NodeTypeStable and NodeTypeSuspect. mu must be held before calling
// this method.
func (n *nodeList) unsafeTakeAndDelete(hash proto.NodeHash, source NodeType) (*proto.Node, bool) {
	var node *proto.Node
	switch source {
	case NodeTypeStable:
		nn, ok := n.stable[hash]
		if !ok {
			return nil, false
		}
		node = nn
		delete(n.stable, hash)
		delete(n.nodeByType, hash)
	case NodeTypeSuspect:
		nn, ok := n.suspects[hash]
		if !ok {
			return nil, false
		}
		delete(n.suspects, hash)
		delete(n.nodeByType, hash)
		node = nn.Node
	default:
		return nil, false
	}

	return node, true
}

func (n *nodeList) MarkStable(hash proto.NodeHash) {
	n.mu.Lock()
	n.mu.Unlock()
	currentState, ok := n.nodeByType[hash]
	if !ok || currentState == NodeTypeStable {
		return
	}

	node, ok := n.unsafeTakeAndDelete(hash, currentState)
	if !ok {
		return
	}
	node.Suspect = false
	n.nodeByType[hash] = NodeTypeStable
	n.stable[hash] = node
}

func (n *nodeList) MarkSuspect(period uint16, remote bool, hash proto.NodeHash) {
	n.mu.Lock()
	n.mu.Unlock()

	currentState, ok := n.nodeByType[hash]
	if !ok || currentState == NodeTypeSuspect {
		return
	}

	node, ok := n.unsafeTakeAndDelete(hash, currentState)
	if !ok {
		return
	}

	node.Suspect = true
	n.nodeByType[hash] = NodeTypeSuspect
	n.suspects[hash] = &Suspect{
		Node:            node,
		SuspectAtPeriod: period,
		FromRemote:      remote,
	}
}

// unsafeMarkFaulty marks a node with given `hash` as Faulty (by removing it
// from the members list). mu must be held before calling this method.
func (n *nodeList) unsafeMarkFaulty(hash proto.NodeHash) {
	currentState, ok := n.nodeByType[hash]
	if !ok {
		n.logger.Debug("Ignoring unsafeMarkFaulty for unknown node",
			zap.String("node hash", hash.String()))
		return
	}

	n.logger.Debug("unsafeTakeAndDelete for",
		zap.String("node hash", hash.String()),
		zap.String("current state", currentState.String()))

	n.unsafeTakeAndDelete(hash, currentState)
}

func (n *nodeList) MarkFaulty(hash proto.NodeHash) {
	n.mu.Lock()
	n.mu.Unlock()
	n.unsafeMarkFaulty(hash)
}

func (n *nodeList) ExpiredSuspects(currentPeriod, expirationRounds uint16) []*proto.Node {
	n.mu.RLock()
	defer n.mu.RUnlock()

	var result []*proto.Node

	for hash, node := range n.suspects {
		if uint16(min(math.Abs(float64(currentPeriod)-float64(node.SuspectAtPeriod)), float64(math.MaxUint16-1))) >= expirationRounds {
			if node.FromRemote {
				n.logger.Debug("Marking node as faulty",
					zap.String("node hash", hash.String()))
				n.unsafeMarkFaulty(hash)
				continue
			}
			result = append(result, node.Node)
		}
	}

	return result
}

func (n *nodeList) Delete(hash proto.NodeHash) {
	n.mu.Lock()
	defer n.mu.Unlock()

	currentType, ok := n.nodeByType[hash]
	if !ok {
		return
	}

	switch currentType {
	case NodeTypeStable:
		n.logger.Debug("Deleting stable node with hash",
			zap.String("hash", hash.String()))
		delete(n.stable, hash)
	case NodeTypeSuspect:
		n.logger.Debug("Deleting suspect node with hash",
			zap.String("hash", hash.String()))
		delete(n.suspects, hash)
	}
	delete(n.nodeByType, hash)
}

func (n *nodeList) Bootstrap(period uint16, list []proto.Node) {
	n.mu.Lock()
	defer n.mu.Unlock()

	for _, v := range list {
		func(node proto.Node) {
			n.unsafeAdd(period, &node)
		}(v)
	}
}

func (n *nodeList) NodeByHash(hash proto.NodeHash) *proto.Node {
	n.mu.RLock()
	defer n.mu.RUnlock()

	kind, ok := n.nodeByType[hash]
	if !ok {
		return nil
	}

	if kind == NodeTypeStable {
		node, ok := n.stable[hash]
		if !ok {
			return nil
		}
		return node
	}

	node, ok := n.suspects[hash]
	if !ok {
		return nil
	}
	return node.Node
}

func (n *nodeList) NodeByIP(ip proto.IP) *proto.Node {
	return n.NodeByHash(ip.IntoHash())
}

func (n *nodeList) IndirectPingCandidates(k int, except proto.NodeHash) []*proto.Node {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if len(n.nodeByType) == 0 {
		return nil
	}

	result := make([]*proto.Node, 0, k)
	for key, source := range n.nodeByType {
		if key == except {
			continue
		}

		switch source {
		case NodeTypeStable:
			result = append(result, n.stable[key])
		case NodeTypeSuspect:
			result = append(result, n.suspects[key].Node)
		}

		if len(result) == k {
			break
		}
	}

	return result
}
