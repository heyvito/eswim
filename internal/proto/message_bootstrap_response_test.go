package proto

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

var bootstrapResponseBytes = hex2Bytes(`CAFE12 0001`)

func TestBootstrapResponse_Encode(t *testing.T) {
	pkt := EncPkt(1, BootstrapResponse{})
	assert.Equal(t, bootstrapResponseBytes, pkt)
}

func TestBootstrapResponse_Decode(t *testing.T) {
	pkt, err := mustParsePacket(t, bootstrapResponseBytes)
	require.NoError(t, err)
	assertHeader(t, pkt, BTRR)
	assert.NotNil(t, pkt.Message)
}
