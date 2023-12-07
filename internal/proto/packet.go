package proto

import (
	"github.com/heyvito/eswim/internal/fsm"
	"net"
	"reflect"
)

// Packet contains a Header and a Message object. Additionally, Packet may
// contain an extra Source, representing the IP from which the message came
// from.
type Packet struct {
	Source  net.IP
	Header  Header
	Message Message
}

func (p Packet) Encode(into []byte) {
	headerSize := p.Header.RequiredSize()
	p.Header.Encode(into)
	p.Message.Encode(into[headerSize:])
}

func (p Packet) RequiredSize() int {
	return p.Header.RequiredSize() +
		p.Message.RequiredSize()
}

func Pkt(incarnation uint16, msg Message) *Packet {
	return &Packet{
		Source: nil,
		Header: Header{
			Magic:       protocolMagic,
			Version:     protocolVersion,
			OpCode:      msg.OpCode(),
			Incarnation: incarnation,
		},
		Message: msg,
	}
}

// fsmStates: packetDecoder header, payload

type packetDecoderState uint8

const (
	packetDecoderStateHeader packetDecoderState = iota
	packetDecoderStatePayload
)

// fsmStatesEnd

type packetDecoderContext struct {
	headerDecoder  *headerDecoderT
	payloadDecoder fsm.AnyDecoder
}

func (p *packetDecoderContext) Reset() {
	p.headerDecoder.Reset()
	p.payloadDecoder = nil
}

func (p *packetDecoderContext) Init() {
	p.headerDecoder = headerDecoder.New()
}

var PacketDecoder = fsm.Def[Packet, packetDecoderState, packetDecoderContext]{
	Feed: func(f *fsm.FSM[Packet, packetDecoderState, packetDecoderContext], state packetDecoderState, ctx *packetDecoderContext, b byte) error {
		switch state {
		case packetDecoderStateHeader:
			head, err := ctx.headerDecoder.Feed(b)
			if err != nil {
				return err
			}

			if head != nil {
				decoder, ok := messageDecoderList[head.OpCode]
				if !ok {
					return NoDecoderError{op: head.OpCode}
				}

				f.Value.Header = *head

				if decoder == nil {
					t, ok := emptyMessageTypes[f.Value.Header.OpCode]
					if ok {
						f.Value.Message = reflect.New(t).Interface().(Message)
					}
					return fsm.Done
				}

				ctx.payloadDecoder = decoder.NewGeneric()
				f.Transition(packetDecoderStatePayload)
			}
		case packetDecoderStatePayload:
			msg, err := ctx.payloadDecoder.FeedAny(b)
			if err != nil {
				return err
			}
			if msg != nil {
				f.Value.Message = msg.(Message)
				return fsm.Done
			}
		}

		return nil
	},
}
