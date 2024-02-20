package core

import (
	"cmp"
	"github.com/heyvito/eswim/internal/containers"
	"github.com/heyvito/eswim/internal/proto"
	"go.uber.org/zap"
	"slices"
	"sync"
)

type gossipItem = containers.Tuple[*proto.Event, int]

// GossipManager implements a mechanism to register and obtain gossips about
// items we received through the piggyback mechanism.
type GossipManager interface {
	// Add attempts to add a given event to the internal queue, depending on the
	// current contents (i.e. if the received event can override an existing event)
	Add(item *proto.Event)

	// Next returns a slice of proto.Event that fits in the provided maxLength.
	// Events returned have their transmission counters automatically incremented,
	// and events transmitted the maximum transmission count are automatically
	// removed.
	Next(maxLength int) []proto.Event

	// AdjustMaxTransmission sets the amount of transmission quantity for next
	// messages to a given integer value. This does not affect messages that were
	// enqueued before the call.
	AdjustMaxTransmission(maxTx int)
	LastGossipAbout(addr proto.IP) *proto.Event
	CurrentGossips() []proto.Event
}

// NewGossipManager returns a new gossipManager that will attempt to retransmit
// known events up to maxTx times.
func NewGossipManager(logger *zap.Logger, maxTx int) GossipManager {
	return &gossipManager{
		maxTx: maxTx,
		queue: make([]*gossipItem, 0, 64),
		log:   logger.Named("gossip_manager"),
	}
}

type gossipManager struct {
	queue []*gossipItem
	mu    sync.Mutex
	maxTx int
	log   *zap.Logger
}

func (g *gossipManager) CurrentGossips() []proto.Event {
	g.mu.Lock()
	defer g.mu.Unlock()
	clone := make([]proto.Event, len(g.queue))
	for i, e := range g.queue {
		clone[i] = *e.First
	}
	return clone
}

func (g *gossipManager) LastGossipAbout(ip proto.IP) *proto.Event {
	for _, v := range g.queue {
		if v.First.Payload.GetSubject().Equal(&ip) {
			return v.First
		}
	}
	return nil
}

// reorder reorders the internal queue, keeping the least transmitted items at
// the front. mu must be held.
func (g *gossipManager) reorder() {
	g.log.Debug("Reordering")
	slices.SortFunc(g.queue, func(a, b *gossipItem) int {
		return cmp.Compare(b.Second, a.Second)
	})
}

// autoReorder is a utility function that returns a signal and a deferred
// function. The deferred function is meant to be passed to defer, and the
// signal function can be called whenever the internal queue is modified.
// When deferred is invoked, if signal has been called, a reorder is executed.
// mu must be held until after the call to deferred.
func (g *gossipManager) autoReorder() (signal func(), deferred func()) {
	changed := false
	signal = func() { changed = true }
	deferred = func() {
		if changed {
			g.reorder()
		}
	}
	return
}

// findEventByNode takes a proto.Event and returns a gossipItem that matches
// the same subject of the provided event. Returns nil in case no event for
// that node is present in the local queue. It is recommended to hold mu during
// this operation.
func (g *gossipManager) findEventByNode(item *proto.Event) *gossipItem {
	itemSubject := item.Payload.GetSubject()
	for _, v := range g.queue {
		if v.First.Payload.GetSubject().Equal(&itemSubject) {
			return v
		}
	}
	return nil
}

func (g *gossipManager) Add(item *proto.Event) {
	g.mu.Lock()
	defer g.mu.Unlock()
	changed, autoReorder := g.autoReorder()
	defer autoReorder()

	if existing := g.findEventByNode(item); existing != nil {
		if !item.Payload.(Invalidator).Invalidates(existing.First) {
			g.log.Debug("Event skipped as it does not invalidate the existing one",
				zap.String("new", item.String()),
				zap.String("existing", existing.First.String()))
			return
		}
		g.log.Debug("Accepted new event",
			zap.String("node", item.Payload.GetSubject().String()),
			zap.String("event", item.String()))
		existing.First = item
		existing.Second = 0
		changed()
		return
	}

	changed()

	t := containers.Tup(item, g.maxTx)
	g.queue = append(g.queue, &t)
}

func (g *gossipManager) Next(maxLength int) []proto.Event {
	g.mu.Lock()
	defer g.mu.Unlock()
	changed, autoReorder := g.autoReorder()
	defer autoReorder()

	currentLen := 0
	var items []proto.Event
	var indexesToRemove []int

	for i, v := range g.queue {
		event, txLeft := v.Decompose()
		eventSize := event.RequiredSize()
		if currentLen+eventSize > maxLength {
			break
		}
		items = append(items, *event)
		currentLen += eventSize

		if txLeft == 1 {
			g.log.Debug("Dropping event", zap.String("event", event.String()))
			indexesToRemove = append(indexesToRemove, i)
		} else {
			v.Second--
		}
	}

	if len(indexesToRemove) > 0 {
		for i := len(indexesToRemove) - 1; i >= 0; i-- {
			g.queue = slices.Delete(g.queue, i, i+1)
		}
		changed()
	}

	return items
}

func (g *gossipManager) AdjustMaxTransmission(maxTx int) {
	if maxTx == g.maxTx {
		return
	}
	g.log.Debug("Adjusting maxTX", zap.Int("new_value", maxTx))
	g.maxTx = maxTx
}
