package proto

import "github.com/heyvito/eswim/internal/fsm"

// Left represents the value of an EventLeft, and indicates that a given node
// Subject intends to leave the cluster immediately. This prevents other nodes
// from wasting resources trying to reach a node that left a cluster in a
// graceful manner.
type Left struct {
	Subject     IP
	Incarnation uint16
}

func (l *Left) RequiredSize() int {
	return 2 + l.Subject.Size()
}

func (l *Left) Encode(into []byte) {
	newWriter(into).
		ip(l.Subject).
		u16(l.Incarnation)
}

func (l *Left) Kind() EventKind                 { return EventLeft }
func (l *Left) SubjectAddressKind() AddressKind { return l.Subject.AddressKind }
func (l *Left) SourceAddressKind() AddressKind  { return AddressIPv4 }
func (l *Left) FromNode(n *Node) {
	copyIP(&l.Subject, &n.Address)
	l.Incarnation = n.Incarnation
}
func (l *Left) GetIncarnation() uint16 { return l.Incarnation }
func (l *Left) GetSubject() IP         { return l.Subject }

// fsmStates: leftDecoder subject, incarnation

type leftDecoderState uint8

const (
	leftDecoderStateSubject leftDecoderState = iota
	leftDecoderStateIncarnation
)

// fsmStatesEnd

type leftDecoderContext struct {
	SubjectAddressKind AddressKind
}

func (l *leftDecoderContext) SetSubjectAddressKind(addr AddressKind) {
	l.SubjectAddressKind = addr
}

var leftDecoder = fsm.Def[Left, leftDecoderState, leftDecoderContext]{
	Prepare: func(f *fsm.FSM[Left, leftDecoderState, leftDecoderContext], state leftDecoderState, ctx *leftDecoderContext) {
		f.Value.Subject.AddressKind = ctx.SubjectAddressKind
		f.Size = ctx.SubjectAddressKind.Size()
		f.SatisfySize()
	},
	Feed: func(f *fsm.FSM[Left, leftDecoderState, leftDecoderContext], state leftDecoderState, ctx *leftDecoderContext, b byte) error {
		switch state {
		case leftDecoderStateSubject:
			fsmCopyIP(f, &f.Value.Subject)
			f.TransitionSatisfySize(leftDecoderStateIncarnation, 2)

		case leftDecoderStateIncarnation:
			f.Value.Incarnation = f.U16()
			return fsm.Done
		}
		return nil
	},
}
