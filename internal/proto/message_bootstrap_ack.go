package proto

// BootstrapAck represents a response for a BootstrapResponse, and contains
// no fields. This message is emitted to the control socket of a node to
// indicate that the bootstrap process can now proceed through the TCP channel.
// This is required to prevent multiple bootstrap responders to try to connect
// to the same node requesting bootstrap.
type BootstrapAck struct{}

func (BootstrapAck) OpCode() OpCode    { return BTRA }
func (BootstrapAck) Encode([]byte)     {}
func (BootstrapAck) RequiredSize() int { return 0 }
