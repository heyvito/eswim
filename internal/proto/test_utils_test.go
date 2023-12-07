package proto

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/heyvito/eswim/internal/fsm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func hex2Bytes(data string) []byte {
	data = strings.ReplaceAll(data, "\n", "")
	data = strings.ReplaceAll(data, " ", "")
	value, err := hex.DecodeString(data)
	if err != nil {
		panic(fmt.Sprintf("Failed reading hex data: %s", err))
	}

	return value
}

func mustParsePacket(t *testing.T, data []byte) (*Packet, error) {
	t.Helper()
	dec := PacketDecoder.New()
	idx := 0
	var pkt *Packet
	var err error
	for i, v := range data {
		idx = i
		pkt, err = dec.Feed(v)
		if err != nil {
			return nil, err
		}
		if pkt != nil {
			break
		}
	}

	if idx != len(data)-1 {
		t.Fatalf("Short read reading packet. Read %d of %d bytes.", idx+1, len(data))
	}

	if pkt == nil {
		t.Fatalf("Decode yielded no packet")
	}

	return pkt, nil
}

func assertHeader(t *testing.T, pkt *Packet, code OpCode) {
	t.Helper()
	assert.Equal(t, protocolMagic, pkt.Header.Magic)
	assert.Equal(t, uint8(1), pkt.Header.Version)
	assert.Equal(t, code, pkt.Header.OpCode)
	assert.Equal(t, uint16(1), pkt.Header.Incarnation)
}

func encodeEncoder(enc Encoder) []byte {
	buf := make([]byte, enc.RequiredSize())
	enc.Encode(buf)
	return buf
}

func decodeInto[T any, S ~uint8, C any](t *testing.T, decoder fsm.Def[T, S, C], data []byte) *T {
	t.Helper()
	dec := decoder.New()
	idx := 0
	var r *T = nil
	var err error
	for i, v := range data {
		idx = i
		r, err = dec.Feed(v)
		require.NoError(t, err)
		if r != nil {
			break
		}
	}

	if idx != len(data)-1 {
		t.Fatalf("Short read: Expected %d bytes to be consumed, but only %d were", len(data), idx+1)
	}

	if r == nil {
		t.Fatal("Unexpected EOF. All bytes were consumed, but no output was received.")
	}

	return r
}

func randomIPv6(t *testing.T) IP {
	ip := IP{AddressKind: AddressIPv6}
	_, err := rand.Read(ip.AddressBytes[:])
	require.NoError(t, err)
	return ip
}

func randomIPv4(t *testing.T) IP {
	ip := IP{AddressKind: AddressIPv4}
	_, err := rand.Read(ip.AddressBytes[:ip.Size()])
	require.NoError(t, err)
	return ip
}

func randomIP(t *testing.T, kind AddressKind) IP {
	if kind == AddressIPv4 {
		return randomIPv4(t)
	}
	return randomIPv6(t)
}

func randomUint16(t *testing.T) uint16 {
	var bytes [2]byte
	_, err := rand.Read(bytes[:])
	require.NoError(t, err)
	return uint16(bytes[0])<<8 | uint16(bytes[1])
}

func ipToHex(ip IP) string {
	return hex.EncodeToString(ip.AddressBytes[:ip.Size()])
}

func u16ToHex(v uint16) string {
	return hex.EncodeToString([]byte{uint8(v >> 8), uint8(v & 0xFF)})
}
