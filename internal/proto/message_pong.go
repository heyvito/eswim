package proto

import "github.com/heyvito/eswim/internal/fsm"

// Pong represents a response for a Ping message.
type Pong struct {
	// Ack indicates whether the target node accepted the ping payload. When set
	// to false, this indicates that the provided Ping.TargetIncarnation did not
	// correspond to the current node's incarnation, and was therefore ignored.
	// This is used to avoid nodes announcing suspects due to their own view
	// being outdated.
	Ack bool

	Cookie, Period uint16

	IsLastStateKnown     bool
	LastKnownEventKind   EventKind
	LastKnownIncarnation uint16
}

func (p Pong) OpCode() OpCode { return PONG }

func (p Pong) RequiredSize() int { return 7 }

func (p Pong) Encode(into []byte) {
	flags := uint8(0x00)
	if p.Ack {
		flags |= 0x1
	}
	if p.IsLastStateKnown {
		flags |= 0x2
		flags |= byte(p.LastKnownEventKind) << 2
	}

	newWriter(into).
		u8(flags).
		u16(p.Cookie).
		u16(p.Period).
		u16(p.LastKnownIncarnation)
}

// fsmStates: pongDecoder flags, cookie, period, lastKnownIncarnation

type pongDecoderState uint8

const (
	pongDecoderStateFlags pongDecoderState = iota
	pongDecoderStateCookie
	pongDecoderStatePeriod
	pongDecoderStateLastKnownIncarnation
)

// fsmStatesEnd

var pongDecoder = fsm.Def[Pong, pongDecoderState, struct{}]{
	Feed: func(f *fsm.FSM[Pong, pongDecoderState, struct{}], state pongDecoderState, ctx *struct{}, b byte) error {
		switch state {
		case pongDecoderStateFlags:
			f.Value.Ack = b&0x1 == 0x1
			f.Value.IsLastStateKnown = b&0x2 == 0x2
			if f.Value.IsLastStateKnown {
				f.Value.LastKnownEventKind = EventKind(b >> 2)
			}
			f.TransitionSatisfySize(pongDecoderStateCookie, 2)

		case pongDecoderStateCookie:
			f.Value.Cookie = f.U16()
			f.TransitionSatisfySize(pongDecoderStatePeriod, 2)

		case pongDecoderStatePeriod:
			f.Value.Period = f.U16()
			f.TransitionSatisfySize(pongDecoderStateLastKnownIncarnation, 2)

		case pongDecoderStateLastKnownIncarnation:
			f.Value.LastKnownIncarnation = f.U16()
			return fsm.Done
		}

		return nil
	},
}
