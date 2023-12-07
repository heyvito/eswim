package proto

import "github.com/heyvito/eswim/internal/fsm"

// TCPPacket represents an opaque TCP message of an arbitrary length. This
// packet is a mere convenience to retain all parsing operations isolated in the
// networking package.
type TCPPacket struct {
	Magic   uint16
	Version uint8
	Payload []byte
}

func (t TCPPacket) RequiredSize() int {
	return 7 + len(t.Payload)
}

func (t TCPPacket) Encode(into []byte) {
	newWriter(into).
		u16(protocolMagic).
		u8(protocolVersion).
		u32(uint32(len(t.Payload))).
		bytes(t.Payload)
}

// fsmStates: tcpPacketDecoder magic, version, size, payload

type tcpPacketDecoderState uint8

const (
	tcpPacketDecoderStateMagic tcpPacketDecoderState = iota
	tcpPacketDecoderStateVersion
	tcpPacketDecoderStateSize
	tcpPacketDecoderStatePayload
)

// fsmStatesEnd

// TCPPacketDecoder is responsible for decoding a TCP stream into TCPPacket(s)
var TCPPacketDecoder = fsm.Def[TCPPacket, tcpPacketDecoderState, struct{}]{
	InitialSize: 2,
	Feed: func(f *fsm.FSM[TCPPacket, tcpPacketDecoderState, struct{}], state tcpPacketDecoderState, ctx *struct{}, b byte) error {
		switch state {
		case tcpPacketDecoderStateMagic:
			if f.U16() != protocolMagic {
				f.Reset()
				return nil
			}
			f.Value.Magic = protocolMagic

			f.TransitionSatisfySize(tcpPacketDecoderStateVersion, 1)

		case tcpPacketDecoderStateVersion:
			if b != 1 {
				f.Reset()
				return nil
			}
			f.Value.Version = b

			f.TransitionSatisfySize(tcpPacketDecoderStateSize, 4)

		case tcpPacketDecoderStateSize:
			f.Value.Payload = make([]byte, f.U32())
			f.TransitionSatisfySize(tcpPacketDecoderStatePayload, len(f.Value.Payload))

		case tcpPacketDecoderStatePayload:
			f.CopyPayload(f.Value.Payload)
			return fsm.Done
		}

		return nil
	},
}.IntoExported()
