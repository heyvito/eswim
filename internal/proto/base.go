package proto

import (
	"bytes"
	"encoding/binary"
	"github.com/heyvito/eswim/internal/fsm"
	"golang.org/x/sys/unix"
	"net"
	"net/netip"
	"reflect"
)

// stringer can be installed from golang.org/x/tools/cmd/stringer@latest
//go:generate stringer -type=OpCode

type OpCode uint8

const (
	// BTRP is a BootstrapRequest message
	BTRP OpCode = 0x01
	// BTRR is a BootstrapResponse message
	BTRR OpCode = 0x02
	// BTRA is a BootstrapAck message
	BTRA OpCode = 0x03
	// PING is a Ping message
	PING OpCode = 0x04
	// PONG is a Pong message
	PONG OpCode = 0x05
	// IING is an IndirectPing message
	IING OpCode = 0x06
	// IONG is an IndirectPong message
	IONG OpCode = 0x07
	// SYNC is a FullSync message
	SYNC OpCode = 0x08

	// When adding new messages, remember to update minOpCode and maxOpCode :)
)

const (
	minOpCode              = BTRP
	maxOpCode              = SYNC
	protocolVersion uint8  = 1
	protocolMagic   uint16 = 0xCAFE
)

// stringer can be installed from golang.org/x/tools/cmd/stringer@latest
//go:generate stringer -type=AddressKind

// AddressKind indicates which kind of address is contained within a message or
// substructure
type AddressKind uint8

// Size returns the amount of bytes required to represent this AddressKind
func (a AddressKind) Size() int {
	if a == AddressIPv4 {
		return 4
	}

	return 16
}

// UnixProto returns the Unix syscall value representing the current AddressKind
func (a AddressKind) UnixProto() int {
	if a == AddressIPv4 {
		return unix.IPPROTO_IP
	}
	return unix.IPPROTO_IPV6
}

const (
	AddressIPv4 AddressKind = 0
	AddressIPv6 AddressKind = 1
)

// IP stores an AddressKind along with the bytes comprising it
type IP struct {
	AddressKind  AddressKind
	AddressBytes [16]byte
}

func (i IP) IntoHash() NodeHash {
	ip, _ := netip.AddrFromSlice(i.Bytes())
	return NodeHash{ip}
}

// IntoManaged returns the net.IP representation for the current IP
func (i IP) IntoManaged() netip.Addr {
	ip, _ := netip.AddrFromSlice(i.Bytes())
	return ip
}

// Size returns the amount of bytes required to represent this IP
func (i IP) Size() int { return i.AddressKind.Size() }

// Bytes returns a slice of bytes representing this IP
func (i IP) Bytes() []byte { return i.AddressBytes[:i.Size()] }

// Equal returns whether the current IP is equal to the provided value
func (i IP) Equal(o *IP) bool {
	if o == nil {
		return false
	}
	return o.AddressKind == i.AddressKind &&
		bytes.Equal(i.AddressBytes[:], o.AddressBytes[:])
}

// String returns the string representation of the current IP
func (i IP) String() string { return i.IntoManaged().String() }

// ipV4 returns a v4 IP based on  bytes provided to it. This function is intended
// // to be used only for testing purposes.
// Panics if len(data) != 4
func ipV4(data ...byte) IP {
	if len(data) != 4 {
		panic("Invalid IP size")
	}
	i := IP{AddressKind: AddressIPv4}
	copy(i.AddressBytes[:], data)
	return i
}

// ipV6 returns a v6 IP based on bytes provided to it. This function is intended
// // to be used only for testing purposes.
// Panics if len(data) != 16
func ipV6(data ...byte) IP {
	if len(data) != 16 {
		panic("Invalid IP size")
	}
	i := IP{AddressKind: AddressIPv6}
	copy(i.AddressBytes[:], data)
	return i
}

// messageDecoderList holds a map associating OpCode to its decoder. OpCodes with nil
// as their values indicates that they do not have a body, hence no decoder
// is needed. However, empty messages must be added to emptyMessageTypes.
var messageDecoderList = map[OpCode]fsm.AnyDefinition{
	BTRP: nil,
	BTRR: nil,
	BTRA: nil,
	PING: pingDecoder,
	PONG: pongDecoder,
	IING: indirectPingDecoder,
	IONG: indirectPongDecoder,
	SYNC: syncDecoder,
}

// emptyMessageTypes associates [OpCode]s of messages lacking a decoder to their
// respective reflect.Type.
var emptyMessageTypes = map[OpCode]reflect.Type{
	BTRP: reflect.TypeOf(BootstrapRequest{}),
	BTRR: reflect.TypeOf(BootstrapResponse{}),
	BTRA: reflect.TypeOf(BootstrapAck{}),
}

// stringer can be installed from golang.org/x/tools/cmd/stringer@latest
//go:generate stringer -type=EventKind

type EventKind uint8

const (
	// EventJoin represents an event indicating that a new member has been added
	// to the cluster after a known member bootstraps it.
	EventJoin EventKind = 0b0000

	// EventHealthy indicates that a cluster member asserted a given node as
	// healthy, after it has been suspected as faulty.
	EventHealthy EventKind = 0b0001

	// EventSuspect indicates that a cluster member suspects that a given node
	// is suspected to be faulty.
	EventSuspect EventKind = 0b0010

	// EventFaulty indicates that a cluster member could not be reached for a
	// given period of time and is no longer considered a member of the current
	// cluster.
	EventFaulty EventKind = 0b0011

	// EventLeft is voluntarily emitted by nodes before they leave the cluster,
	// potentially avoiding a whole suspect->indirect ping->faulty check by
	// other cluster members.
	EventLeft EventKind = 0b0100

	// EventAlive is emitted by nodes that noticed their healthiness was being
	// suspected, in order to indicate that it is actually running and able to
	// be part of the cluster.
	EventAlive EventKind = 0b0101

	// EventWrap is emitted by nodes to indicate that their incarnation count
	// will wrap back to zero after exhausting other uint16 values.
	EventWrap EventKind = 0b0110
)

// eventDecoderList associates each EventKind with their respective FSM decoder.
var eventDecoderList = map[EventKind]fsm.AnyDefinition{
	EventJoin:    joinDecoder,
	EventSuspect: suspectDecoder,
	EventFaulty:  faultyDecoder,
	EventLeft:    leftDecoder,
	EventAlive:   aliveDecoder,
	EventWrap:    wrapDecoder,
	EventHealthy: healthyDecoder,
}

// Message abstracts relevant methods of a message
type Message interface {
	OpCode() OpCode
	Encoder
}

// Encoder represents any structure that can be encoded by Writer or any
// other decoding facility
type Encoder interface {
	// RequiredSize returns the amount of bytes required to encode the current
	// structure.
	RequiredSize() int

	// Encode takes a slice of bytes and writes the current structure's
	// serialized representation into it. It assumes that len(into) is equal or
	// greater to the value returned by RequiredSize.
	Encode(into []byte)
}

// EventEncoder represents a structure capable of encoding an Event
type EventEncoder interface {
	Encoder
	Kind() EventKind
	SourceAddressKind() AddressKind
	SubjectAddressKind() AddressKind
	GetIncarnation() uint16
	GetSubject() IP
	Invalidates(other *Event) bool
}

type EventWithSource interface {
	GetSource() IP
}

var (
	u16Marshal = binary.BigEndian.PutUint16
	u32Marshal = binary.BigEndian.PutUint32
)

// reduce performs a functional reduce operation on a given set of items T,
// returning the result as a single U.
func reduce[T, U any](arr []T, fn func(i T, acc U) U) U {
	var acc U
	for _, v := range arr {
		acc = fn(v, acc)
	}
	return acc
}

// forEachCast executes a forEach operation on a given set, casting the input
// type T to U before invoking fn. This function does not perform checked casts,
// meaning it will panic in case T cannot be cast to U.
func forEachCast[T any, U any](in []T, fn func(i U)) {
	for _, v := range in {
		var y any = v
		fn(y.(U))
	}
}

// EncTCPPkt encodes a given Message into a TCPPacket, sealing it using the
// provided `cryptoSealer`. `incarnation` and `message` are handled verbatim by
// EncPkt before sealing.
func EncTCPPkt(incarnation uint16, message Message, cryptoSealer func([]byte) ([]byte, error)) (*TCPPacket, error) {
	buf := EncPkt(incarnation, message)
	buf, err := cryptoSealer(buf)
	if err != nil {
		return nil, err
	}
	return &TCPPacket{
		Magic:   protocolMagic,
		Version: protocolVersion,
		Payload: buf,
	}, nil
}

// EncPkt encodes a given Message with a given incarnation identifier into a
// Packet, and returns its bytes.
func EncPkt(incarnation uint16, message Message) []byte {
	pkt := Pkt(incarnation, message)
	data := make([]byte, pkt.RequiredSize())
	pkt.Header.Encode(data)
	pkt.Message.Encode(data[pkt.Header.RequiredSize():])
	return data
}

// fsmCopyIP is a utility function responsible for copying the current payload
// present in a given FSM `f` to an IP structure, taking into account its
// size, that must have already been set beforehand.
func fsmCopyIP[T any, S ~uint8, C any](f *fsm.FSM[T, S, C], into *IP) {
	copy(into.AddressBytes[:], f.Payload[:into.Size()])
}

// copyIP is a utility function that copies a given IP address src into dst. dst
// will have its kind changed to reflect src's kind.
func copyIP(dst, src *IP) {
	clear(dst.AddressBytes[:])
	dst.AddressKind = src.AddressKind
	copy(dst.AddressBytes[:], src.AddressBytes[:src.Size()])
}

// NodeInheritor represents a structure that can update its own values given
// a Node instance.
type NodeInheritor interface {
	FromNode(n *Node)
}

// IPFromAddr returns a new IP from a given netip.Addr structure.
func IPFromAddr(addr netip.Addr) IP {
	i := IP{}
	if addr.Is6() {
		i.AddressKind = AddressIPv6
		addr16 := addr.As16()
		copy(i.AddressBytes[:], addr16[:])
	} else {
		i.AddressKind = AddressIPv4
		addr16 := addr.As4()
		copy(i.AddressBytes[:], addr16[:])
	}

	return i
}

// IPFromNetIP returns a new IP from a given net.IP structure.
func IPFromNetIP(ip net.IP) (out IP) {
	if v4 := ip.To4(); v4 != nil {
		copy(out.AddressBytes[:], v4[:4])
		return
	}

	out.AddressKind = AddressIPv6
	copy(out.AddressBytes[:], ip.To16())
	return
}

// ParsePacket attempts to parse a provided data byte slice into a Packet
// structure, including its payload. Returns an error in case the operation
// does not yield a valid Packet.
func ParsePacket(data []byte) (*Packet, error) {
	dec := PacketDecoder.New()
	for _, b := range data {
		pkt, err := dec.Feed(b)
		if err != nil {
			return nil, err
		}
		if pkt != nil {
			return pkt, nil
		}
	}

	return nil, CouldNotParsePacketErr
}
