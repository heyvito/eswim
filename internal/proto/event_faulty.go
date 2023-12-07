package proto

import "github.com/heyvito/eswim/internal/fsm"

// Faulty represents the value of an EventFaulty, and indicates that Source
// assumed { Subject, inc=Incarnation } as faulty due to lack of response and
// failure to receive responses from indirect pings.
type Faulty struct {
	Source      IP
	Subject     IP
	Incarnation uint16
}

func (f *Faulty) RequiredSize() int {
	return 2 + f.Source.Size() + f.Subject.Size()
}

func (f *Faulty) Encode(into []byte) {
	newWriter(into).
		ip(f.Source).
		ip(f.Subject).
		u16(f.Incarnation)
}

func (f *Faulty) Kind() EventKind                 { return EventFaulty }
func (f *Faulty) SourceAddressKind() AddressKind  { return f.Source.AddressKind }
func (f *Faulty) SubjectAddressKind() AddressKind { return f.Subject.AddressKind }
func (f *Faulty) FromNode(n *Node) {
	f.Incarnation = n.Incarnation
	copyIP(&f.Subject, &n.Address)
}
func (f *Faulty) SetSource(addr *IP)     { copyIP(&f.Source, addr) }
func (f *Faulty) GetIncarnation() uint16 { return f.Incarnation }
func (f *Faulty) GetSubject() IP         { return f.Subject }

// fsmStates: faultyDecoder source, subject, incarnation

type faultyDecoderState uint8

const (
	faultyDecoderStateSource faultyDecoderState = iota
	faultyDecoderStateSubject
	faultyDecoderStateIncarnation
)

// fsmStatesEnd

type faultyDecoderContext struct {
	SourceAddressKind, SubjectAddressKind AddressKind
}

func (f *faultyDecoderContext) SetSubjectAddressKind(addr AddressKind) {
	f.SubjectAddressKind = addr
}

func (f *faultyDecoderContext) SetSourceAddressKind(addr AddressKind) {
	f.SourceAddressKind = addr
}

var faultyDecoder = fsm.Def[Faulty, faultyDecoderState, faultyDecoderContext]{
	Prepare: func(f *fsm.FSM[Faulty, faultyDecoderState, faultyDecoderContext], state faultyDecoderState, ctx *faultyDecoderContext) {
		f.Value.Subject.AddressKind = ctx.SubjectAddressKind
		f.Value.Source.AddressKind = ctx.SourceAddressKind
		f.Size = ctx.SourceAddressKind.Size()
		f.SatisfySize()
	},
	Feed: func(f *fsm.FSM[Faulty, faultyDecoderState, faultyDecoderContext], state faultyDecoderState, ctx *faultyDecoderContext, b byte) error {
		switch state {
		case faultyDecoderStateSource:
			fsmCopyIP(f, &f.Value.Source)
			f.TransitionSatisfySize(faultyDecoderStateSubject, ctx.SubjectAddressKind.Size())

		case faultyDecoderStateSubject:
			fsmCopyIP(f, &f.Value.Subject)
			f.TransitionSatisfySize(faultyDecoderStateIncarnation, 2)

		case faultyDecoderStateIncarnation:
			f.Value.Incarnation = f.U16()
			return fsm.Done
		}

		return nil
	},
}
