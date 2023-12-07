package proto

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

var (
	indirectPingV4Bytes = hex2Bytes("CAFE 16 0001 00 04D2 3039 01020304")
	indirectPingV6Bytes = hex2Bytes("CAFE 16 0001 01 04D2 3039 000102030405060708090A0B0C0D0E0F")
)

var (
	indirectPingV4 = IndirectPing{
		Cookie:  1234,
		Period:  12345,
		Address: ipV4(0x01, 0x02, 0x03, 0x04),
	}
	indirectPingV6 = IndirectPing{
		Cookie:  1234,
		Period:  12345,
		Address: ipV6(0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F),
	}
)

func TestIndirectPing_Encode(t *testing.T) {
	t.Parallel()
	t.Run("ipV4", func(t *testing.T) {
		data := EncPkt(1, indirectPingV4)
		assert.Equal(t, indirectPingV4Bytes, data)
	})
	t.Run("ipV6", func(t *testing.T) {
		data := EncPkt(1, indirectPingV6)
		assert.Equal(t, indirectPingV6Bytes, data)
	})
}

func TestIndirectPing_Decode(t *testing.T) {
	t.Parallel()
	t.Run("ipV4", func(t *testing.T) {
		pkt, err := mustParsePacket(t, indirectPingV4Bytes)
		require.NoError(t, err)
		assertHeader(t, pkt, IING)
		assert.Equal(t, &indirectPingV4, pkt.Message.(*IndirectPing))
	})

	t.Run("ipV6", func(t *testing.T) {
		pkt, err := mustParsePacket(t, indirectPingV6Bytes)
		require.NoError(t, err)
		assertHeader(t, pkt, IING)
		assert.Equal(t, &indirectPingV6, pkt.Message.(*IndirectPing))
	})
}
