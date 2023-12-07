package core

import (
	cryptoRand "crypto/rand"
	"github.com/heyvito/eswim/internal/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	mathRand "math/rand"
	"testing"
)

func makeIP(t *testing.T) (ip proto.IP) {
	ip.AddressKind = proto.AddressKind(mathRand.Intn(1))
	_, err := cryptoRand.Read(ip.AddressBytes[:ip.Size()])
	require.NoError(t, err)
	return
}

func makeUint16() uint16 {
	return uint16(mathRand.Int())
}

func makeGossipManager(maxTx int) *gossipManager {
	return NewGossipManager(zap.NewNop(), maxTx).(*gossipManager)
}

func makeEvent(t *testing.T, kind ...proto.EventKind) *proto.Event {
	var eventKind proto.EventKind
	if len(kind) == 0 {
		eventKind = proto.EventKind(mathRand.Intn(7))
	} else {
		eventKind = kind[0]
	}

	var ev proto.EventEncoder
	switch eventKind {
	case proto.EventJoin:
		ev = &proto.Join{
			Source:      makeIP(t),
			Subject:     makeIP(t),
			Incarnation: makeUint16(),
		}
	case proto.EventHealthy:
		ev = &proto.Healthy{
			Source:      makeIP(t),
			Subject:     makeIP(t),
			Incarnation: makeUint16(),
		}
	case proto.EventSuspect:
		ev = &proto.Suspect{
			Source:      makeIP(t),
			Subject:     makeIP(t),
			Incarnation: makeUint16(),
		}
	case proto.EventFaulty:
		ev = &proto.Faulty{
			Source:      makeIP(t),
			Subject:     makeIP(t),
			Incarnation: makeUint16(),
		}
	case proto.EventLeft:
		ev = &proto.Left{
			Subject:     makeIP(t),
			Incarnation: makeUint16(),
		}
	case proto.EventAlive:
		ev = &proto.Alive{
			Subject:     makeIP(t),
			Incarnation: makeUint16(),
		}
	case proto.EventWrap:
		ev = &proto.Wrap{
			Subject:        makeIP(t),
			Incarnation:    makeUint16() + 1,
			NewIncarnation: 0,
		}
	default:
		panic("wtf")
	}

	return &proto.Event{Payload: ev}
}

func TestGossipManager_Add(t *testing.T) {
	t.Run("Add single", func(t *testing.T) {
		gm := makeGossipManager(3)
		ev := makeEvent(t)
		gm.Add(ev)
		assert.Len(t, gm.queue, 1)
	})

	t.Run("New entries are added to the front of transmitted ones", func(t *testing.T) {
		gm := makeGossipManager(3)
		gm.Add(makeEvent(t))
		gm.queue[0].Second = 1
		ev := makeEvent(t)
		gm.Add(ev)
		assert.Len(t, gm.queue, 2)
		assert.Equal(t, ev, gm.queue[0].First)
	})

	t.Run("Overrides events as per invalidation rules", func(t *testing.T) {
		// Invalidation rules are thoroughly on proto/invalidation_test.go. Here
		// we just want to assert that invalidation is respected by leveraging
		// Faulty always invalidating everything.
		gm := makeGossipManager(3)
		ev1 := makeEvent(t, proto.EventHealthy)
		gm.Add(ev1)
		assert.Len(t, gm.queue, 1)
		gm.queue[0].Second = 1

		ev2 := makeEvent(t, proto.EventFaulty)
		ip1 := ev1.Payload.(*proto.Healthy).Subject
		ip2 := &ev2.Payload.(*proto.Faulty).Subject
		ip2.AddressKind = ip1.AddressKind
		copy(ip2.AddressBytes[:], ip1.AddressBytes[:])
		gm.Add(ev2)
		assert.Len(t, gm.queue, 1)
		assert.Equal(t, ev2, gm.queue[0].First)
		assert.Zero(t, gm.queue[0].Second)
	})
}

func TestGossipManager_Next(t *testing.T) {
	t.Run("returns fitting", func(t *testing.T) {
		gm := makeGossipManager(3)
		ev1 := makeEvent(t)
		ev2 := makeEvent(t)
		ev3 := makeEvent(t)
		gm.Add(ev1)
		gm.Add(ev2)
		gm.Add(ev3)

		reqLen := ev1.RequiredSize() + ev2.RequiredSize() + ev3.RequiredSize() - 1
		next := gm.Next(reqLen)
		assert.Len(t, next, 2)
	})

	t.Run("decrements transmission counter", func(t *testing.T) {
		gm := makeGossipManager(3)
		ev := makeEvent(t)
		gm.Add(ev)
		reqLen := ev.RequiredSize()
		gm.Next(reqLen)
		assert.Equal(t, 2, gm.queue[0].Second)
	})

	t.Run("removes items transmitted enough", func(t *testing.T) {
		gm := makeGossipManager(3)
		ev := makeEvent(t)
		gm.Add(ev)
		reqLen := ev.RequiredSize()
		q := gm.Next(reqLen)
		assert.Len(t, q, 1)
		assert.Len(t, gm.queue, 1)

		q = gm.Next(reqLen)
		assert.Len(t, q, 1)
		assert.Len(t, gm.queue, 1)

		q = gm.Next(reqLen)
		assert.Len(t, q, 1)
		assert.Len(t, gm.queue, 0)
	})
}
