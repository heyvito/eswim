package core

import (
	"github.com/heyvito/eswim/internal/proto"
	"github.com/heyvito/eswim/internal/wire"
	"time"
)

var defaultNowFn = func() int64 { return time.Now().UTC().UnixMilli() }

type Invalidator interface {
	Invalidates(ev *proto.Event) bool
}

type TCPPacketEncoder interface {
	EncodeTCPPacket(message proto.Message) (*proto.TCPPacket, error)
}

type TCPPacketDecoder interface {
	DecodeTCPPacket(tcpPkt *proto.TCPPacket, client wire.TCPControlClient) (*proto.Packet, error)
}
