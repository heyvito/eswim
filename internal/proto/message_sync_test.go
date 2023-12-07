package proto

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

var syncBytes = hex2Bytes(`
CAFE 18 0001
0104
00 0001 01020304
01 0005 000102030405060708090A0B0C0D0E0F
02 0002 05060708
03 0006 101112131415161718191A1B1C1D1E1F
`)

var syncBytesEmpty = hex2Bytes(`
CAFE 18 0001
0100
`)

var syncRequest = Sync{
	Mode: SyncModeSecondPhase,
	Nodes: []Node{
		ipv4Healthy,
		ipv6Healthy,
		ipv4Suspect,
		ipv6Suspect,
	},
}

var syncRequestEmpty = Sync{
	Mode:  SyncModeSecondPhase,
	Nodes: []Node{},
}

func TestSync_Encode(t *testing.T) {
	data := EncPkt(1, syncRequest)
	assert.Equal(t, syncBytes, data)
}

func TestSync_Decode(t *testing.T) {
	pkt, err := mustParsePacket(t, syncBytes)
	require.NoError(t, err)
	assertHeader(t, pkt, SYNC)
	assert.Equal(t, &syncRequest, pkt.Message.(*Sync))
}

func TestSync_DecodeEmpty(t *testing.T) {
	pkt, err := mustParsePacket(t, syncBytesEmpty)
	require.NoError(t, err)
	assertHeader(t, pkt, SYNC)
	assert.Equal(t, &syncRequestEmpty, pkt.Message.(*Sync))
}
