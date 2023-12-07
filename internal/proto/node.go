package proto

import (
	"fmt"
	"github.com/heyvito/eswim/internal/fsm"
	"net/netip"
)

// NodeHash is a container type responsible for representing a Node unique
// identifier, which for our purposes, is the identity of the Node's IP address
// bytes.
type NodeHash struct{ netip.Addr }

var NodeHashZero = NodeHash{}

// Node represents a member of the cluster.
type Node struct {
	/// Suspect denotes whether a node, as perceived by the transmitting node,
	// is potentially in a StateFaulty condition.
	Suspect bool

	// Incarnation represents the last known incarnation ID of this node.
	Incarnation uint16

	// Address represents the address of this member.
	Address IP
}

func (n Node) String() string {
	return fmt.Sprintf("{Node Suspect:%t, Incarnation: %d, Address: %#v}",
		n.Suspect, n.Incarnation, n.Address.String())
}

func (n Node) RequiredSize() int {
	return 3 + n.Address.Size()
}

func (n Node) Encode(into []byte) {
	flags := byte(0x00)
	if n.Address.AddressKind == AddressIPv6 {
		flags |= 0x1
	}
	if n.Suspect {
		flags |= 0x2
	}

	newWriter(into).
		u8(flags).
		u16(n.Incarnation).
		ip(n.Address)
}

// Equal determines whether a given Node is equal another given Node.
func (n Node) Equal(other Node) bool {
	return n.Address == other.Address &&
		n.Suspect == other.Suspect &&
		n.Incarnation == other.Incarnation &&
		n.Address.Equal(&other.Address)
}

// Hash returns a hashed representation of the current node's address in the
// form of a NodeHash
func (n Node) Hash() NodeHash {
	return n.Address.IntoHash()
}

// fsmStates: nodeDecoder flags, incarnation, address

type nodeDecoderState uint8

const (
	nodeDecoderStateFlags nodeDecoderState = iota
	nodeDecoderStateIncarnation
	nodeDecoderStateAddress
)

// fsmStatesEnd

type nodeDecoderT = fsm.FSM[Node, nodeDecoderState, struct{}]

var nodeDecoder = fsm.Def[Node, nodeDecoderState, struct{}]{
	Feed: func(f *fsm.FSM[Node, nodeDecoderState, struct{}], state nodeDecoderState, ctx *struct{}, b byte) error {
		switch state {
		case nodeDecoderStateFlags:
			f.Value.Address.AddressKind = AddressKind(b & 0x01)
			f.Value.Suspect = b&0x02 == 0x02
			f.TransitionSatisfySize(nodeDecoderStateIncarnation, 2)
		case nodeDecoderStateIncarnation:
			f.Value.Incarnation = f.U16()
			f.TransitionSatisfySize(nodeDecoderStateAddress, f.Value.Address.Size())
		case nodeDecoderStateAddress:
			fsmCopyIP(f, &f.Value.Address)
			return fsm.Done
		}

		return nil

	},
}
