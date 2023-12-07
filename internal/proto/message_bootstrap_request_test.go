package proto

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

var bootstrapRequestBytes = hex2Bytes(`CAFE11 0001`)

func TestBootstrapRequest_Encode(t *testing.T) {
	pkt := EncPkt(1, BootstrapRequest{})
	assert.Equal(t, bootstrapRequestBytes, pkt)
}

func TestBootstrapRequest_Decode(t *testing.T) {
	pkt, err := mustParsePacket(t, bootstrapRequestBytes)
	require.NoError(t, err)
	assertHeader(t, pkt, BTRP)
	assert.NotNil(t, pkt.Message)
}
