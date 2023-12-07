package proto

import (
	"github.com/heyvito/eswim/internal/fsm"
)

// Ping represents a ping request to another cluster member.
type Ping struct {
	// Cookie represents a random value representing this ping request
	Cookie uint16
	// Period represents the current protocol period in the member that
	// generated this message.
	Period uint16
	// TargetIncarnation indicates the known incarnation of the node being
	// pinged in the current view of the local node
	TargetIncarnation uint16

	// Events contains a list of events observed by the node sending this ping.
	Events []Event
}

func (p Ping) OpCode() OpCode { return PING }

func (p Ping) RequiredSize() int {
	eventsSize := reduce(p.Events, func(i Event, acc int) int {
		return acc + i.RequiredSize()
	})
	return 7 + eventsSize
}

func (p Ping) Encode(into []byte) {
	w := newWriter(into).
		u16(p.Cookie).
		u16(p.Period).
		u16(p.TargetIncarnation).
		u8(uint8(len(p.Events)))
	forEachCast(p.Events, w.encoderNoChain)
}

// fsmStates: pingDecoder cookie, period, targetIncarnation, eventSize, events

type pingDecoderState uint8

const (
	pingDecoderStateCookie pingDecoderState = iota
	pingDecoderStatePeriod
	pingDecoderStateTargetIncarnation
	pingDecoderStateEventSize
	pingDecoderStateEvents
)

// fsmStatesEnd

type pingDecoderContext struct {
	eventDecoder *eventDecoderT
}

func (p *pingDecoderContext) Init() {
	p.eventDecoder = eventDecoder.New()
}

func (p *pingDecoderContext) Reset() {
	p.eventDecoder.Reset()
}

var pingDecoder = fsm.Def[Ping, pingDecoderState, pingDecoderContext]{
	InitialSize: 2,
	Feed: func(f *fsm.FSM[Ping, pingDecoderState, pingDecoderContext], state pingDecoderState, ctx *pingDecoderContext, b byte) error {
		switch state {
		case pingDecoderStateCookie:
			f.Value.Cookie = f.U16()
			f.TransitionSatisfySize(pingDecoderStatePeriod, 2)

		case pingDecoderStatePeriod:
			f.Value.Period = f.U16()
			f.TransitionSatisfySize(pingDecoderStateTargetIncarnation, 2)

		case pingDecoderStateTargetIncarnation:
			f.Value.TargetIncarnation = f.U16()
			f.Transition(pingDecoderStateEventSize)

		case pingDecoderStateEventSize:
			qty := int(b)
			if qty == 0 {
				return fsm.Done
			}

			f.TransitionSize(pingDecoderStateEvents, qty)

		case pingDecoderStateEvents:
			ev, err := ctx.eventDecoder.Feed(b)
			if err != nil {
				return err
			}
			if ev != nil {
				f.Size--
				f.Value.Events = append(f.Value.Events, *ev)
			}

			if f.Size != 0 {
				break
			}

			return fsm.Done
		}

		return nil
	},
}
