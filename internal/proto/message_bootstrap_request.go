package proto

// BootstrapRequest represents a bootstrap message emitted in a multicast
// network. Other nodes listening to the same network may respond to this
// message by emitting a UDP Unicast BootstrapResponse.
type BootstrapRequest struct{}

func (BootstrapRequest) OpCode() OpCode    { return BTRP }
func (BootstrapRequest) Encode([]byte)     {}
func (BootstrapRequest) RequiredSize() int { return 0 }
