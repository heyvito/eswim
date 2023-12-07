package proto

import "github.com/heyvito/eswim/internal/fsm"

// Suspect represents the value of an EventSuspect, which indicates that a
// node Source could not reach a second node Subject, and started indirect
// Ping attempts.
type Suspect struct {
	Source      IP
	Subject     IP
	Incarnation uint16
}

func (s *Suspect) RequiredSize() int {
	return 2 + s.Source.Size() + s.Subject.Size()
}

func (s *Suspect) Encode(into []byte) {
	newWriter(into).
		ip(s.Source).
		ip(s.Subject).
		u16(s.Incarnation)
}

func (s *Suspect) Kind() EventKind                 { return EventSuspect }
func (s *Suspect) SourceAddressKind() AddressKind  { return s.Source.AddressKind }
func (s *Suspect) SubjectAddressKind() AddressKind { return s.Subject.AddressKind }
func (s *Suspect) SetSource(source *IP)            { copyIP(&s.Source, source) }
func (s *Suspect) FromNode(n *Node) {
	copyIP(&s.Subject, &n.Address)
	s.Incarnation = n.Incarnation
}
func (s *Suspect) GetIncarnation() uint16 { return s.Incarnation }
func (s *Suspect) GetSubject() IP         { return s.Subject }

// fsmStates: suspectDecoder source, subject, incarnation

type suspectDecoderState uint8

const (
	suspectDecoderStateSource suspectDecoderState = iota
	suspectDecoderStateSubject
	suspectDecoderStateIncarnation
)

// fsmStatesEnd

type suspectDecoderContext struct {
	SourceAddressKind, SubjectAddressKind AddressKind
}

func (s *suspectDecoderContext) SetSubjectAddressKind(addr AddressKind) {
	s.SubjectAddressKind = addr
}

func (s *suspectDecoderContext) SetSourceAddressKind(addr AddressKind) {
	s.SourceAddressKind = addr
}

var suspectDecoder = fsm.Def[Suspect, suspectDecoderState, suspectDecoderContext]{
	Prepare: func(f *fsm.FSM[Suspect, suspectDecoderState, suspectDecoderContext], state suspectDecoderState, ctx *suspectDecoderContext) {
		f.Value.Subject.AddressKind = ctx.SubjectAddressKind
		f.Value.Source.AddressKind = ctx.SourceAddressKind
		f.Size = ctx.SourceAddressKind.Size()
		f.SatisfySize()
	},
	Feed: func(f *fsm.FSM[Suspect, suspectDecoderState, suspectDecoderContext], state suspectDecoderState, ctx *suspectDecoderContext, b byte) error {
		switch state {
		case suspectDecoderStateSource:
			f.Value.Source.AddressKind = ctx.SourceAddressKind
			fsmCopyIP(f, &f.Value.Source)
			f.TransitionSatisfySize(suspectDecoderStateSubject, ctx.SubjectAddressKind.Size())
		case suspectDecoderStateSubject:
			f.Value.Subject.AddressKind = ctx.SubjectAddressKind
			fsmCopyIP(f, &f.Value.Subject)
			f.TransitionSatisfySize(suspectDecoderStateIncarnation, 2)
		case suspectDecoderStateIncarnation:
			f.Value.Incarnation = f.U16()
			return fsm.Done
		}
		return nil
	},
}
