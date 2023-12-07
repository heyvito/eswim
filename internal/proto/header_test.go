package proto

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

var headerBytes = hex2Bytes(`CAFE11 0001`)

func TestHeader_Encode(t *testing.T) {
	h := Header{
		Magic:       protocolMagic,
		Version:     protocolVersion,
		OpCode:      BTRP,
		Incarnation: 1,
	}
	data := encodeEncoder(h)
	assert.Equal(t, headerBytes, data)
}

func TestHeader_Decode(t *testing.T) {
	dec := decodeInto(t, headerDecoder, headerBytes)

	h := Header{
		Magic:       protocolMagic,
		Version:     protocolVersion,
		OpCode:      BTRP,
		Incarnation: 1,
	}
	assert.Equal(t, &h, dec)
}
