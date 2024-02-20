package proto

import "github.com/heyvito/eswim/internal/fsm"

// Join represents the value of an EventJoin, and indicates that a node Source
// has allowed{ Subject, inc = Incarnation } to join the current cluster.
type Join struct {
	Source      IP
	Subject     IP
	Incarnation uint16
}

func (j *Join) RequiredSize() int {
	return 2 + j.Source.Size() + j.Subject.Size()
}

func (j *Join) Encode(into []byte) {
	newWriter(into).
		ip(j.Source).
		ip(j.Subject).
		u16(j.Incarnation)
}

func (j *Join) Kind() EventKind                 { return EventJoin }
func (j *Join) SourceAddressKind() AddressKind  { return j.Source.AddressKind }
func (j *Join) SubjectAddressKind() AddressKind { return j.Subject.AddressKind }
func (j *Join) FromNode(n *Node) {
	j.Incarnation = n.Incarnation
	copyIP(&j.Subject, &n.Address)
}
func (j *Join) SetSource(addr *IP)     { copyIP(&j.Source, addr) }
func (j *Join) GetIncarnation() uint16 { return j.Incarnation }
func (j *Join) GetSubject() IP         { return j.Subject }
func (j *Join) GetSource() IP          { return j.Source }

// fsmStates: joinDecoder source, subject, incarnation

type joinDecoderState uint8

const (
	joinDecoderStateSource joinDecoderState = iota
	joinDecoderStateSubject
	joinDecoderStateIncarnation
)

// fsmStatesEnd

type joinDecoderContext struct {
	SourceAddressKind, SubjectAddressKind AddressKind
}

func (j *joinDecoderContext) SetSubjectAddressKind(addr AddressKind) {
	j.SubjectAddressKind = addr
}

func (j *joinDecoderContext) SetSourceAddressKind(addr AddressKind) {
	j.SourceAddressKind = addr
}

var joinDecoder = fsm.Def[Join, joinDecoderState, joinDecoderContext]{
	Prepare: func(f *fsm.FSM[Join, joinDecoderState, joinDecoderContext], state joinDecoderState, ctx *joinDecoderContext) {
		f.Value.Subject.AddressKind = ctx.SubjectAddressKind
		f.Value.Source.AddressKind = ctx.SourceAddressKind
		f.Size = ctx.SourceAddressKind.Size()
		f.SatisfySize()
	},
	Feed: func(f *fsm.FSM[Join, joinDecoderState, joinDecoderContext], state joinDecoderState, ctx *joinDecoderContext, b byte) error {
		switch state {
		case joinDecoderStateSource:
			fsmCopyIP(f, &f.Value.Source)
			f.TransitionSatisfySize(joinDecoderStateSubject, ctx.SubjectAddressKind.Size())

		case joinDecoderStateSubject:
			fsmCopyIP(f, &f.Value.Subject)
			f.TransitionSatisfySize(joinDecoderStateIncarnation, 2)

		case joinDecoderStateIncarnation:
			f.Value.Incarnation = f.U16()
			return fsm.Done
		}
		return nil
	},
}
