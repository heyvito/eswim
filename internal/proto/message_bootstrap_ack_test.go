package proto

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

var bootstrapAckBytes = hex2Bytes(`CAFE13 0001`)

func TestBootstrapAck_Encode(t *testing.T) {
	pkt := EncPkt(1, BootstrapAck{})
	assert.Equal(t, bootstrapAckBytes, pkt)
}

func TestBootstrapAck_Decode(t *testing.T) {
	pkt, err := mustParsePacket(t, bootstrapAckBytes)
	require.NoError(t, err)
	assertHeader(t, pkt, BTRA)
	assert.NotNil(t, pkt.Message)
}
