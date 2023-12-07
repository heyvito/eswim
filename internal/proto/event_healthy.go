package proto

import "github.com/heyvito/eswim/internal/fsm"

// Healthy represents the value of an EventHealthy and indicates that Source
// assumed { Subject, inc=Incarnation } as healthy, after a protocol period.
type Healthy struct {
	Source      IP
	Subject     IP
	Incarnation uint16
}

func (h *Healthy) RequiredSize() int               { return 2 + h.Subject.Size() + h.Source.Size() }
func (h *Healthy) Kind() EventKind                 { return EventHealthy }
func (h *Healthy) SourceAddressKind() AddressKind  { return h.Source.AddressKind }
func (h *Healthy) Encode(into []byte)              { newWriter(into).ip(h.Source).ip(h.Subject).u16(h.Incarnation) }
func (h *Healthy) SubjectAddressKind() AddressKind { return h.Subject.AddressKind }
func (h *Healthy) SetSource(ip *IP)                { copyIP(&h.Subject, ip) }
func (h *Healthy) GetIncarnation() uint16          { return h.Incarnation }
func (h *Healthy) GetSubject() IP                  { return h.Subject }

func (h *Healthy) FromNode(n *Node) {
	copyIP(&h.Subject, &n.Address)
	h.Incarnation = n.Incarnation
}

// fsmStates: healthyDecoder source, subject, incarnation

type healthyDecoderState uint8

const (
	healthyDecoderStateSource healthyDecoderState = iota
	healthyDecoderStateSubject
	healthyDecoderStateIncarnation
)

// fsmStatesEnd

type healthyDecoderContext struct {
	SubjectAddressKind AddressKind
	SourceAddressKind  AddressKind
}

func (a *healthyDecoderContext) SetSourceAddressKind(addr AddressKind) {
	a.SourceAddressKind = addr
}

func (a *healthyDecoderContext) SetSubjectAddressKind(addr AddressKind) {
	a.SubjectAddressKind = addr
}

var healthyDecoder = fsm.Def[Healthy, healthyDecoderState, healthyDecoderContext]{
	Prepare: func(f *fsm.FSM[Healthy, healthyDecoderState, healthyDecoderContext], state healthyDecoderState, ctx *healthyDecoderContext) {
		f.Value.Subject.AddressKind = ctx.SubjectAddressKind
		f.Value.Source.AddressKind = ctx.SourceAddressKind
		f.Size = ctx.SourceAddressKind.Size()
		f.SatisfySize()
	},
	Feed: func(f *fsm.FSM[Healthy, healthyDecoderState, healthyDecoderContext], state healthyDecoderState, ctx *healthyDecoderContext, b byte) error {
		switch state {
		case healthyDecoderStateSource:
			fsmCopyIP(f, &f.Value.Source)
			f.TransitionSatisfySize(healthyDecoderStateSubject, ctx.SubjectAddressKind.Size())

		case healthyDecoderStateSubject:
			fsmCopyIP(f, &f.Value.Subject)
			f.TransitionSatisfySize(healthyDecoderStateIncarnation, 2)

		case healthyDecoderStateIncarnation:
			f.Value.Incarnation = f.U16()
			return fsm.Done
		}
		return nil
	},
}
