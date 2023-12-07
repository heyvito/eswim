package proto

import (
	"encoding/hex"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"math/rand"
	"testing"
)

var eventKinds = []EventKind{EventJoin, EventHealthy, EventSuspect, EventFaulty, EventLeft, EventAlive, EventWrap}

func makeEvents(t *testing.T) (Ping, []byte) {
	expectedEvents := ""
	counter := 0
	var tmpBuffer []byte
	var allEvents []Event

	for _, sourceKind := range []AddressKind{0, 1} {
		for _, subjectKind := range []AddressKind{0, 1} {
			sourceKind := sourceKind
			kinds := make([]EventKind, len(eventKinds))
			copy(kinds, eventKinds)
			rand.Shuffle(len(kinds), func(i, j int) {
				kinds[i], kinds[j] = kinds[j], kinds[i]
			})

			for _, k := range kinds {
				counter++

				eventToAppend := ""
				var encoder EventEncoder
				switch k {
				case EventJoin:
					ev := &Join{
						Source:      randomIP(t, sourceKind),
						Subject:     randomIP(t, subjectKind),
						Incarnation: randomUint16(t),
					}
					encoder = ev
					eventToAppend = ipToHex(ev.Source) + ipToHex(ev.Subject) + u16ToHex(ev.Incarnation)

				case EventHealthy:
					ev := &Healthy{
						Source:      randomIP(t, sourceKind),
						Subject:     randomIP(t, subjectKind),
						Incarnation: randomUint16(t),
					}
					encoder = ev
					eventToAppend = ipToHex(ev.Source) + ipToHex(ev.Subject) + u16ToHex(ev.Incarnation)

				case EventSuspect:
					ev := &Suspect{
						Source:      randomIP(t, sourceKind),
						Subject:     randomIP(t, subjectKind),
						Incarnation: randomUint16(t),
					}
					encoder = ev
					eventToAppend = ipToHex(ev.Source) + ipToHex(ev.Subject) + u16ToHex(ev.Incarnation)

				case EventFaulty:
					ev := &Faulty{
						Source:      randomIP(t, sourceKind),
						Subject:     randomIP(t, subjectKind),
						Incarnation: randomUint16(t),
					}
					encoder = ev
					eventToAppend = ipToHex(ev.Source) + ipToHex(ev.Subject) + u16ToHex(ev.Incarnation)

				case EventLeft:
					sourceKind = AddressIPv4
					ev := &Left{
						Subject:     randomIP(t, subjectKind),
						Incarnation: randomUint16(t),
					}
					encoder = ev
					eventToAppend = ipToHex(ev.Subject) + u16ToHex(ev.Incarnation)

				case EventAlive:
					sourceKind = AddressIPv4
					ev := &Alive{
						Subject:     randomIP(t, subjectKind),
						Incarnation: randomUint16(t),
					}
					encoder = ev
					eventToAppend = ipToHex(ev.Subject) + u16ToHex(ev.Incarnation)

				case EventWrap:
					sourceKind = AddressIPv4
					ev := &Wrap{
						Subject:        randomIP(t, subjectKind),
						Incarnation:    randomUint16(t),
						NewIncarnation: randomUint16(t),
					}
					encoder = ev
					eventToAppend = ipToHex(ev.Subject) + u16ToHex(ev.Incarnation) + u16ToHex(ev.NewIncarnation)
				}

				flags := uint8(0x00)

				if sourceKind == AddressIPv6 {
					flags |= 0x8
				}
				if subjectKind == AddressIPv6 {
					flags |= 0x4
				}
				flags |= uint8(k) << 4
				tmpBuffer = make([]byte, encoder.RequiredSize())
				encoder.Encode(tmpBuffer)
				allEvents = append(allEvents, Event{Payload: encoder})
				expectedEvents += fmt.Sprintf("%02x", flags) + eventToAppend
			}
		}
	}

	ping := Ping{
		Cookie:            randomUint16(t),
		Period:            randomUint16(t),
		TargetIncarnation: randomUint16(t),
		Events:            allEvents,
	}
	expectedEncode := "CAFE 14 0001 " +
		u16ToHex(ping.Cookie) +
		u16ToHex(ping.Period) +
		u16ToHex(ping.TargetIncarnation) +
		hex.EncodeToString([]byte{byte(counter)}) +
		expectedEvents

	expectedBytes := hex2Bytes(expectedEncode)
	return ping, expectedBytes
}

func TestPing_Encode(t *testing.T) {
	ping, expectedBytes := makeEvents(t)
	currentBytes := EncPkt(1, &ping)
	assert.Equal(t, expectedBytes, currentBytes)
}

func TestPing_Decode(t *testing.T) {
	ping, expectedBytes := makeEvents(t)
	dec, err := mustParsePacket(t, expectedBytes)
	require.NoError(t, err)

	assertHeader(t, dec, PING)

	assert.Equal(t, &ping, dec.Message)
}
