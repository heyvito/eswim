package proto

import "github.com/heyvito/eswim/internal/fsm"

// Header is a structure present in every exchanged message. It contains the
// protocol magic bytes, the version to which the message belongs to, the
// message type, and the incarnation identifier of the member that emitted this
// it.
type Header struct {
	Magic       uint16
	Version     uint8
	OpCode      OpCode
	Incarnation uint16
}

func (h Header) Encode(into []byte) {
	u16Marshal(into, h.Magic)
	into[2] = (h.Version << 4) | byte(h.OpCode)
	u16Marshal(into[3:], h.Incarnation)
}

func (h Header) RequiredSize() int { return 5 }

// fsmStates: headerDecoder magic, versionOpCode, incarnation

type headerDecoderState uint8

const (
	headerDecoderStateMagic headerDecoderState = iota
	headerDecoderStateVersionOpCode
	headerDecoderStateIncarnation
)

// fsmStatesEnd

type headerDecoderT = fsm.FSM[Header, headerDecoderState, struct{}]

var headerDecoder = fsm.Def[Header, headerDecoderState, struct{}]{
	InitialSize: 2,
	Feed: func(f *fsm.FSM[Header, headerDecoderState, struct{}], state headerDecoderState, ctx *struct{}, b byte) error {
		switch state {
		case headerDecoderStateMagic:
			f.Value.Magic = f.U16()
			if f.Value.Magic != protocolMagic {
				defer f.Reset()
				return nil
			}
			f.Transition(headerDecoderStateVersionOpCode)

		case headerDecoderStateVersionOpCode:
			f.Value.Version = (b & 0xF0) >> 4
			f.Value.OpCode = OpCode(b & 0x0F)

			if f.Value.Version != protocolVersion {
				defer f.Reset()
				return nil
			}

			if f.Value.OpCode < minOpCode || f.Value.OpCode > maxOpCode {
				defer f.Reset()
				return nil
			}

			f.TransitionSatisfySize(headerDecoderStateIncarnation, 2)

		case headerDecoderStateIncarnation:
			f.Value.Incarnation = f.U16()
			return fsm.Done
		}

		return nil
	},
}
