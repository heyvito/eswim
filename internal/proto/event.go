package proto

import (
	"fmt"
	"github.com/heyvito/eswim/internal/fsm"
)

type sourceAddrKindSetter interface {
	SetSourceAddressKind(addr AddressKind)
}

type subjectAddrKindSetter interface {
	SetSubjectAddressKind(addr AddressKind)
}

// Event represents a single event that occurred against a given IP.
type Event struct {
	Payload EventEncoder
}

func (e Event) String() string {
	return fmt.Sprintf("Event{Payload:{Kind: %s, Subject: %s, Incarnation: %d}}", e.Payload.Kind().String(), e.Payload.GetSubject().String(), e.Payload.GetIncarnation())
}

// EventKind returns the associated EventKind from the current payload.
func (e Event) EventKind() EventKind { return e.Payload.Kind() }

func (e Event) Encode(into []byte) {
	flags := byte(e.Payload.Kind()) << 4
	if e.Payload.SourceAddressKind() == AddressIPv6 {
		flags |= 0x8
	}
	if e.Payload.SubjectAddressKind() == AddressIPv6 {
		flags |= 0x4
	}

	newWriter(into).
		u8(flags).
		encoder(e.Payload)
}

func (e Event) RequiredSize() int {
	return 1 + e.Payload.RequiredSize()
}

// fsmStates: eventDecoder flags, payload

type eventDecoderState uint8

const (
	eventDecoderStateFlags eventDecoderState = iota
	eventDecoderStatePayload
)

// fsmStatesEnd

type eventDecoderContext struct {
	sourceAddressKind  AddressKind
	subjectAddressKind AddressKind
	eventKind          EventKind
	decoder            fsm.AnyDecoder
}

func (e *eventDecoderContext) Reset() {
	e.sourceAddressKind = AddressIPv4
	e.subjectAddressKind = AddressIPv4
	e.decoder = nil
}

type eventDecoderT = fsm.FSM[Event, eventDecoderState, eventDecoderContext]

var eventDecoder = fsm.Def[Event, eventDecoderState, eventDecoderContext]{
	Feed: func(f *fsm.FSM[Event, eventDecoderState, eventDecoderContext], state eventDecoderState, ctx *eventDecoderContext, b byte) error {
		switch state {
		case eventDecoderStateFlags:
			ctx.eventKind = EventKind((b >> 4) & 0xF)
			ctx.sourceAddressKind = AddressKind((b & 0x8) >> 3)
			ctx.subjectAddressKind = AddressKind((b & 0x4) >> 2)

			decoderDef, ok := eventDecoderList[ctx.eventKind]
			if !ok {
				f.Reset()
				return NoEventDecoderError{DecoderID: uint8(ctx.eventKind)}
			}
			decoder := decoderDef.NewGeneric()
			if ctxAccessor, ok := decoder.(fsm.ContextAccessor); ok {
				innerCtx := ctxAccessor.Context()
				if s, ok := innerCtx.(sourceAddrKindSetter); ok {
					s.SetSourceAddressKind(ctx.sourceAddressKind)
				}
				if s, ok := innerCtx.(subjectAddrKindSetter); ok {
					s.SetSubjectAddressKind(ctx.subjectAddressKind)
				}
			}
			decoder.Prepare()
			ctx.decoder = decoder
			f.Transition(eventDecoderStatePayload)

		case eventDecoderStatePayload:
			//fmt.Printf("Feeding %#v\n", ctx.decoder)
			val, err := ctx.decoder.FeedAny(b)
			if err != nil {
				return err
			}
			if val != nil {
				f.Value.Payload = val.(EventEncoder)
				return fsm.Done
			}
		}

		return nil
	},
}
