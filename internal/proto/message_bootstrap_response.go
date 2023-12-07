package proto

// BootstrapResponse represents a response for a BootstrapRequest, and contains
// no fields. The node's address can be obtained through the Packet structure
// wrapping this BootstrapResponse, and the responding node's incarnation number
// can be obtained from Header.Incarnation, also present in the upper Packet
// structure.
type BootstrapResponse struct{}

func (BootstrapResponse) OpCode() OpCode    { return BTRR }
func (BootstrapResponse) Encode([]byte)     {}
func (BootstrapResponse) RequiredSize() int { return 0 }
