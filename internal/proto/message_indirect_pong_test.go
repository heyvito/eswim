package proto

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

var (
	indirectPongV4Bytes = hex2Bytes("CAFE 17 0001 04D2 3039")
	indirectPongV6Bytes = hex2Bytes("CAFE 17 0001 04D2 3039")
)

var (
	indirectPongV4 = IndirectPong{
		Cookie: 1234,
		Period: 12345,
	}
	indirectPongV6 = IndirectPong{
		Cookie: 1234,
		Period: 12345,
	}
)

func TestIndirectPong_Encode(t *testing.T) {
	t.Parallel()
	t.Run("ipV4", func(t *testing.T) {
		data := EncPkt(1, indirectPongV4)
		assert.Equal(t, indirectPongV4Bytes, data)
	})
	t.Run("ipV6", func(t *testing.T) {
		data := EncPkt(1, indirectPongV6)
		assert.Equal(t, indirectPongV6Bytes, data)
	})
}

func TestIndirectPong_Decode(t *testing.T) {
	t.Parallel()
	t.Run("ipV4", func(t *testing.T) {
		pkt, err := mustParsePacket(t, indirectPongV4Bytes)
		require.NoError(t, err)
		assertHeader(t, pkt, IONG)
		assert.Equal(t, &indirectPongV4, pkt.Message.(*IndirectPong))
	})

	t.Run("ipV6", func(t *testing.T) {
		pkt, err := mustParsePacket(t, indirectPongV6Bytes)
		require.NoError(t, err)
		assertHeader(t, pkt, IONG)
		assert.Equal(t, &indirectPongV6, pkt.Message.(*IndirectPong))
	})
}
