package proto

import "github.com/heyvito/eswim/internal/fsm"

// Wrap represents the value of an EventWrap. It indicates that a given node
// Subject is about to have its incarnation number wrapped to a previous
// value, as a new interaction would overflow the size of the current numeric
// type.
type Wrap struct {
	Subject        IP
	Incarnation    uint16
	NewIncarnation uint16
}

func (w *Wrap) RequiredSize() int               { return 4 + w.Subject.Size() }
func (w *Wrap) Kind() EventKind                 { return EventWrap }
func (w *Wrap) SourceAddressKind() AddressKind  { return AddressIPv4 }
func (w *Wrap) SubjectAddressKind() AddressKind { return w.Subject.AddressKind }
func (w *Wrap) GetIncarnation() uint16          { return w.Incarnation }
func (w *Wrap) GetSubject() IP                  { return w.Subject }
func (w *Wrap) Encode(into []byte) {
	newWriter(into).ip(w.Subject).u16(w.Incarnation).u16(w.NewIncarnation)
}

func (w *Wrap) FromNode(n *Node) {
	copyIP(&w.Subject, &n.Address)
	w.Incarnation = n.Incarnation
}

// fsmStates: wrapDecoder subject, incarnation, newIncarnation

type wrapDecoderState uint8

const (
	wrapDecoderStateSubject wrapDecoderState = iota
	wrapDecoderStateIncarnation
	wrapDecoderStateNewIncarnation
)

// fsmStatesEnd

type wrapDecoderContext struct {
	SubjectAddressKind AddressKind
}

func (w *wrapDecoderContext) SetSubjectAddressKind(addr AddressKind) {
	w.SubjectAddressKind = addr
}

var wrapDecoder = fsm.Def[Wrap, wrapDecoderState, wrapDecoderContext]{
	Prepare: func(f *fsm.FSM[Wrap, wrapDecoderState, wrapDecoderContext], state wrapDecoderState, ctx *wrapDecoderContext) {
		f.Value.Subject.AddressKind = ctx.SubjectAddressKind
		f.Size = ctx.SubjectAddressKind.Size()
		f.SatisfySize()
	},
	Feed: func(f *fsm.FSM[Wrap, wrapDecoderState, wrapDecoderContext], state wrapDecoderState, ctx *wrapDecoderContext, b byte) error {
		switch state {
		case wrapDecoderStateSubject:
			fsmCopyIP(f, &f.Value.Subject)
			f.TransitionSatisfySize(wrapDecoderStateIncarnation, 2)

		case wrapDecoderStateIncarnation:
			f.Value.Incarnation = f.U16()
			f.TransitionSatisfySize(wrapDecoderStateNewIncarnation, 2)

		case wrapDecoderStateNewIncarnation:
			f.Value.NewIncarnation = f.U16()
			return fsm.Done
		}

		return nil
	},
}
