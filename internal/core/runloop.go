package core

import (
	"github.com/heyvito/eswim/internal/proto"
	"github.com/influxdata/tdigest"
	"go.uber.org/zap"
	"math"
	"math/rand"
	"net"
	"sync"
	"time"
)

type RunLoop interface {
	// Start starts the RunLoop in another routine, servicing messages, and
	// performing all protocol operations.
	Start()

	// ReceiveMessage enqueues a given message to be processed by the RunLoop.
	ReceiveMessage(packet *proto.Packet)

	// Shutdown signals that the current RunLoop should stop in the next
	// iteration. This method blocks until the loop has stopped.
	Shutdown()

	// CurrentPeriod returns the current protocol period identifier. This
	// operation is not atomic.
	CurrentPeriod() uint16
}

type RunLoopDelegate interface {
	// NextProbeSubject returns the next node to be probed as part of the
	// protocol period.
	NextProbeSubject() *proto.Node

	// GossipEvents returns the next list of events that should be relayed to
	// the node being probed.
	GossipEvents(maxLen int) []proto.Event

	// DispatchMessage dispatches a given proto.Message to the provided
	// destination.
	DispatchMessage(dst net.IP, message proto.Message)

	// SignalHealthyNode signals that a node identified by the provided
	// proto.NodeHash has successfully been probed by the current protocol
	// period.
	SignalHealthyNode(node proto.NodeHash)

	// SignalSuspectNode signals that a node identified by the provided
	// proto.NodeHash has not met the current protocol period expectations and
	// must be considered suspect.
	SignalSuspectNode(node proto.NodeHash)

	// ProcessExpiredSuspects requests the delegate to process the current
	// suspects list in order to locate nodes that have expired the allowed
	// suspect timeout.
	ProcessExpiredSuspects()

	// PingTimeout specifies the maximum duration to wait for a ping
	// acknowledgment from a node. If this duration is exceeded without
	// receiving an acknowledgment, the node is marked as suspect. This delegate
	// will not be called if Options.UseAdaptivePingTimeout is enabled.
	PingTimeout() time.Duration

	// IndirectPingMembers returns a list of nodes to be used to probe a given
	// node identified by the provided proto.NodeHash
	IndirectPingMembers(node proto.NodeHash) []*proto.Node

	// NodeByHash returns a node identified under a given proto.NodeHash
	NodeByHash(node proto.NodeHash) *proto.Node

	// PerformFullSync instructs the delegate to perform a full sync with any
	// known member of the cluster
	PerformFullSync()
}

// NewRunLoop returns a new RunLoop
func NewRunLoop(log *zap.Logger, protocolPeriod time.Duration, delegate RunLoopDelegate, udpMaxLen int, useAdaptivePing bool, syncProtoRounds int) RunLoop {
	r := &runLoop{
		log:              log.With(zap.String("facility", "runloop")),
		running:          false,
		incomingMessages: make(chan *proto.Packet, 10),
		period: &runLoopPeriod{
			id:       1,
			duration: protocolPeriod,
		},
		delegate:        delegate,
		udpMaxLen:       udpMaxLen,
		done:            make(chan bool),
		adaptivePings:   useAdaptivePing,
		syncProtoRounds: syncProtoRounds,
	}
	if r.adaptivePings {
		r.tDigest = tdigest.New()
		r.pingTimestamps = &sync.Map{}
	}
	return r
}

type runLoopPeriod struct {
	// id represents the current period identifier. This is monotonically
	// increased until it reaches the maximum value of its underlying numeric
	// type. Once this happens, the identifier is reset back to its zero value.
	id uint16

	// acked represents whether the local node has received an ack from
	// node, either directly or indirectly.
	acked bool

	// node represents the identifier of the node being probed during
	// this protocol period.
	node proto.NodeHash

	// cookie represents the current random cookie used for emitting
	// messages to other nodes.
	cookie uint16

	// timer represents the timer responsible for indicating that the current
	// protocol period has elapsed
	timer *time.Timer

	// pingTimeout represents the timer responsible for indicating that the
	// allowed time span for receiving a pong from the current node being probed
	// has elapsed; it should be considered suspect, and indirect pings should
	// be requested to available nodes.
	pingTimeout *time.Timer

	// periodLength represents how long a protocol period lasts. This is the amount of
	// time between the protocol start and end, and how long other housekeeping
	// operations will take place.
	duration time.Duration

	// periodHash represents the union of the period's id and cookie. Used to
	// represent a specific run on a limited time span
	periodHash uint32

	mu sync.Mutex
}

// lock locks the internal mutex to prevent concurrent changes to the current
// period representation.
func (r *runLoopPeriod) lock() { r.mu.Lock() }

// unlock unlocks the internal mutex (once it is held by calling lock)
func (r *runLoopPeriod) unlock() { r.mu.Unlock() }

// incrementID safely increments the period identifier in order to avoid
// overflows. mu must be held to perform this operation.
func (r *runLoopPeriod) unsafeIncrementID() {
	if int(r.id)+1 >= math.MaxUint16 {
		r.id = 0
		return
	}

	r.id++
}

// resetPeriodTimer either resets or initializes the internal period timer.
// mu must not be held.
func (r *runLoopPeriod) resetPeriodTimer() {
	if r.timer == nil {
		r.timer = time.NewTimer(r.duration)
		return
	}
	r.timer.Reset(r.duration)
}

// resetPeriodTimer either resets or initializes the internal ping timeout timer
// to the provided time.Duration. mu must not be held.
func (r *runLoopPeriod) resetPingTimeout(duration time.Duration) {
	if r.pingTimeout == nil {
		r.pingTimeout = time.NewTimer(duration)
		return
	}
	r.pingTimeout.Reset(duration)
}

type runLoop struct {
	// log is the configured logger to emit any messages
	log *zap.Logger

	// running indicates whether the loop is running.
	running bool

	// incomingMessages represents a channel of messages that have been received
	// by downstream mechanisms.
	incomingMessages chan *proto.Packet

	// period contains information and state about how periods are handled. See
	// runloopPeriod.
	period *runLoopPeriod

	// delegate represents any object implementing a RunLoopDelegate to which
	// this instance will exchange information and invoke delegate methods
	// on certain events take place.
	delegate RunLoopDelegate

	// udpMaxLen represents the maximum UDP packet size we can send over the
	// current network.
	udpMaxLen int

	// done is a non-buffered channel, used only to indicate when the loop has
	// stopped.
	done chan bool

	// Adaptive Ping Handler

	// tDigest is a tdigest.TDigest instance responsible for calculating
	// quantiles we will use in case adaptivePings is enabled. This will be nil
	// in case the aforementioned variable is set to false.
	tDigest *tdigest.TDigest

	// adaptivePings defines whether this instance should automatically
	// determine how long to wait for pongs.
	adaptivePings bool

	// pingTimestamps temporarily stores timestamps for pings sent, when
	// adaptivePings is enabled. Otherwise, it will be left uninitialized.
	pingTimestamps *sync.Map

	// See [Options.FullSyncProtocolRounds]
	syncProtoRounds int
}

func (r *runLoop) CurrentPeriod() uint16 { return r.period.id }

func (r *runLoop) Start() {
	if r.running {
		return
	}
	r.running = true
	go r.loop()
}

func (r *runLoop) ReceiveMessage(message *proto.Packet) {
	r.incomingMessages <- message
}

func (r *runLoop) makeCookie() {
	r.period.cookie = uint16(rand.Int())
	r.period.periodHash = uint32(r.period.cookie) | (uint32(r.period.id) << 16)
}

func (r *runLoop) startPeriod() {
	r.period.lock()
	defer r.period.unlock()

	r.period.acked = false
	r.period.node = proto.NodeHashZero
	node := r.delegate.NextProbeSubject()
	if node == nil {
		return
	}

	r.makeCookie()
	r.period.node = node.Hash()
	ping := &proto.Ping{
		Cookie:            r.period.cookie,
		Period:            r.period.id,
		TargetIncarnation: node.Incarnation,
	}
	maxGossipsSize := r.udpMaxLen - ping.RequiredSize()
	ping.Events = r.delegate.GossipEvents(maxGossipsSize)
	r.registerSentPing()
	r.delegate.DispatchMessage(node.Address.Bytes(), ping)
	r.log.Debug("Pinging node", zap.String("addr", node.Address.String()))
}

func (r *runLoop) endPeriod() {
	r.period.lock()
	defer r.period.unlock()

	if r.period.node != proto.NodeHashZero {
		if r.period.acked {
			r.delegate.SignalHealthyNode(r.period.node)
		} else {
			r.delegate.SignalSuspectNode(r.period.node)
		}
	}

	r.period.node = proto.NodeHashZero
	r.delegate.ProcessExpiredSuspects()
	r.period.unsafeIncrementID()
	if r.syncProtoRounds > 0 && (int(r.period.id)%r.syncProtoRounds) == 0 {
		r.delegate.PerformFullSync()
	}
}

func (r *runLoop) registerSentPing() {
	if !r.adaptivePings {
		return
	}
	r.pingTimestamps.Store(r.period.periodHash, time.Now().UTC())
}

func (r *runLoop) registerReceivedPong(pong *proto.Pong) {
	if !r.adaptivePings {
		return
	}
	now := time.Now().UTC()
	id := uint32(pong.Cookie) | (uint32(pong.Period) << 16)
	old, ok := r.pingTimestamps.LoadAndDelete(id)
	if !ok {
		// it may have been garbage-collected.
		return
	}
	r.tDigest.Add(float64(now.Sub(old.(time.Time)).Milliseconds()), 1)
}

func (r *runLoop) pingTimeout() time.Duration {
	if !r.adaptivePings {
		return r.delegate.PingTimeout()
	}

	// In case we are running adaptive, and for any weird reason
	// there's no centroid registered, fallback to half of the protocol
	// period.
	if r.tDigest.Count() == 0 {
		return r.period.duration / 2
	}

	// Return the 99th percentile from all pings we exchanged in the network.
	return min(time.Duration(r.tDigest.Quantile(0.99))*time.Millisecond, r.period.duration)
}

func (r *runLoop) waitPeriod() {
	r.period.resetPeriodTimer()
	r.period.resetPingTimeout(r.pingTimeout())

loop:
	for {
		select {
		case <-r.period.timer.C:
			break loop
		case <-r.period.pingTimeout.C:
			if r.period.node == proto.NodeHashZero {
				continue
			}
			r.pingIndirect()
		case msg := <-r.incomingMessages:
			r.handleMessage(msg)
		}
	}

	if !r.period.pingTimeout.Stop() {
		select {
		case <-r.period.pingTimeout.C:
		default:
		}
	}
}

func (r *runLoop) pingIndirect() {
	r.period.lock()
	defer r.period.unlock()
	if r.adaptivePings {
		r.pingTimestamps.Delete(r.period.periodHash)
	}
	if r.period.acked {
		return
	}

	node := r.delegate.NodeByHash(r.period.node)
	// Does the node still exist?
	if node == nil {
		r.log.Debug("Current probe target was removed before indirect ping could be performed.")
		return
	}

	indirect := r.delegate.IndirectPingMembers(r.period.node)
	if len(indirect) == 0 {
		return
	}

	r.log.Debug("Performing indirect ping",
		zap.String("target", node.Address.String()))

	for _, v := range indirect {
		r.delegate.DispatchMessage(v.Address.Bytes(), &proto.IndirectPing{
			Cookie:  r.period.cookie,
			Period:  r.period.id,
			Address: node.Address,
		})
	}
}

func (r *runLoop) handleMessage(pkt *proto.Packet) {
	switch msg := pkt.Message.(type) {
	case *proto.Pong:
		r.period.lock()
		defer r.period.unlock()

		if msg.Period != r.period.id {
			r.log.Debug("Dropping message with incorrect period",
				zap.Uint16("expected", r.period.id),
				zap.Uint16("received", msg.Period))
			return
		}

		if msg.Cookie != r.period.cookie {
			return
		}

		r.registerReceivedPong(msg)

		r.period.acked = true

	case *proto.IndirectPong:
		r.period.lock()
		defer r.period.unlock()

		if msg.Period != r.period.id {
			r.log.Debug("Dropping message with incorrect period",
				zap.Uint16("expected", r.period.id),
				zap.Uint16("received", msg.Period))
			return
		}

		if msg.Cookie != r.period.cookie {
			return
		}
		r.period.acked = true
	default:
		r.log.Warn("RunLoop cannot handle delegated packet",
			zap.String("opcode", pkt.Header.OpCode.String()))
	}
}

func (r *runLoop) loop() {
	for r.running {
		if r.period.id%10 == 0 {
			r.log.Debug("Start period", zap.Uint16("id", r.period.id))
		}
		r.startPeriod()
		r.waitPeriod()
		r.endPeriod()
	}
	close(r.done)
}

func (r *runLoop) Shutdown() {
	r.running = false
	<-r.done
}
