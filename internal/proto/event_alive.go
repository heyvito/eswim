package proto

import "github.com/heyvito/eswim/internal/fsm"

// Alive represents the value of an EventAlive event. It refutes a Suspect
// event, given that { Alive.Incarnation=j, Suspect.Incarnation=i }, j > i.
type Alive struct {
	Subject     IP
	Incarnation uint16
}

func (a *Alive) RequiredSize() int { return 2 + a.Subject.Size() }

func (a *Alive) Encode(into []byte) {
	newWriter(into).ip(a.Subject).u16(a.Incarnation)
}

func (a *Alive) Kind() EventKind { return EventAlive }

func (a *Alive) SourceAddressKind() AddressKind { return AddressIPv4 }

func (a *Alive) SubjectAddressKind() AddressKind {
	return a.Subject.AddressKind
}

func (a *Alive) FromNode(n *Node) {
	copyIP(&a.Subject, &n.Address)
	a.Incarnation = n.Incarnation
}

func (a *Alive) GetIncarnation() uint16 { return a.Incarnation }
func (a *Alive) GetSubject() IP         { return a.Subject }

// fsmStates: aliveDecoder subject, incarnation

type aliveDecoderState uint8

const (
	aliveDecoderStateSubject aliveDecoderState = iota
	aliveDecoderStateIncarnation
)

// fsmStatesEnd

type aliveDecoderContext struct {
	SubjectAddressKind AddressKind
}

func (a *aliveDecoderContext) SetSubjectAddressKind(addr AddressKind) {
	a.SubjectAddressKind = addr
}

var aliveDecoder = fsm.Def[Alive, aliveDecoderState, aliveDecoderContext]{
	Prepare: func(f *fsm.FSM[Alive, aliveDecoderState, aliveDecoderContext], state aliveDecoderState, ctx *aliveDecoderContext) {
		f.Value.Subject.AddressKind = ctx.SubjectAddressKind
		f.Size = ctx.SubjectAddressKind.Size()
		f.SatisfySize()
	},
	Feed: func(f *fsm.FSM[Alive, aliveDecoderState, aliveDecoderContext], state aliveDecoderState, ctx *aliveDecoderContext, b byte) error {
		switch state {
		case aliveDecoderStateSubject:
			fsmCopyIP(f, &f.Value.Subject)
			f.TransitionSatisfySize(aliveDecoderStateIncarnation, 2)

		case aliveDecoderStateIncarnation:
			f.Value.Incarnation = f.U16()
			return fsm.Done
		}
		return nil
	},
}
