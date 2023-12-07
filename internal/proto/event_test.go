package proto

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

var (
	eventBytes = hex2Bytes("14 01020304 000102030405060708090A0B0C0D0E0F 0001")
	event      = Event{
		Payload: &Healthy{
			Source:      ipV4(0x01, 0x02, 0x03, 0x04),
			Subject:     ipV6(0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F),
			Incarnation: 1,
		},
	}
)

func TestEvent_Encode(t *testing.T) {
	assert.Equal(t, eventBytes, encodeEncoder(event))
}

func TestEvent_Decode(t *testing.T) {
	assert.Equal(t, &event, decodeInto(t, eventDecoder, eventBytes))
}
