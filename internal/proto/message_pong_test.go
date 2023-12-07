package proto

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

var pongBytes = hex2Bytes("CAFE 15 0001 1B 04D2 3039 0AA7")
var pong = Pong{
	Cookie:               1234,
	Period:               12345,
	Ack:                  true,
	IsLastStateKnown:     true,
	LastKnownEventKind:   EventWrap,
	LastKnownIncarnation: 2727,
}

func TestPong_Encode(t *testing.T) {
	buf := EncPkt(1, pong)
	assert.Equal(t, pongBytes, buf)
}

func TestPong_Decode(t *testing.T) {
	pkt, err := mustParsePacket(t, pongBytes)
	require.NoError(t, err)
	assertHeader(t, pkt, PONG)
	assert.Equal(t, &pong, pkt.Message.(*Pong))
}
