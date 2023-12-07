package proto

import "github.com/heyvito/eswim/internal/fsm"

// IndirectPong represents a response for a IndirectPing
type IndirectPong struct {
	Cookie uint16
	Period uint16
}

func (i IndirectPong) OpCode() OpCode { return IONG }

func (i IndirectPong) RequiredSize() int { return 4 }

func (i IndirectPong) Encode(into []byte) {
	newWriter(into).
		u16(i.Cookie).
		u16(i.Period)
}

// fsmStates: indirectPongDecoder cookie, period

type indirectPongDecoderState uint8

const (
	indirectPongDecoderStateCookie indirectPongDecoderState = iota
	indirectPongDecoderStatePeriod
)

// fsmStatesEnd

var indirectPongDecoder = fsm.Def[IndirectPong, indirectPongDecoderState, struct{}]{
	InitialSize: 2,
	Feed: func(f *fsm.FSM[IndirectPong, indirectPongDecoderState, struct{}], state indirectPongDecoderState, ctx *struct{}, b byte) error {
		switch state {
		case indirectPongDecoderStateCookie:
			f.Value.Cookie = f.U16()
			f.TransitionSatisfySize(indirectPongDecoderStatePeriod, 2)
		case indirectPongDecoderStatePeriod:
			f.Value.Period = f.U16()
			return fsm.Done
		}
		return nil
	},
}
