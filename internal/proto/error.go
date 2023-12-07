package proto

import "fmt"

// CouldNotParsePacketErr indicates that a parsing operation could not yield
// a valid result from the provided data buffer.
var CouldNotParsePacketErr = fmt.Errorf("could not parse data")

// NoDecoderError indicates that this library does not have a decoder for a
// given received message.
type NoDecoderError struct {
	op OpCode
}

func (n NoDecoderError) Error() string {
	return fmt.Sprintf("No decoder available for OpCode 0x%02x (%s)", n.op, n.op.String())
}

// NoEventDecoderError indicates that this library does not have a decoder for
// given received event.
type NoEventDecoderError struct {
	DecoderID uint8
}

func (n NoEventDecoderError) Error() string {
	return fmt.Sprintf("No event decoder available for event type 0x%02x", n.DecoderID)
}
