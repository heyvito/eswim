package eswim

import (
	"fmt"
	"github.com/heyvito/eswim/internal/containers"
	"github.com/heyvito/eswim/internal/core"
	"github.com/heyvito/eswim/internal/iputil"
	"github.com/heyvito/eswim/internal/logutil"
	"github.com/heyvito/eswim/internal/proto"
	"github.com/heyvito/eswim/internal/wire"
	"go.uber.org/zap"
	"math"
	"math/rand"
	"net"
	"net/netip"
	"sync/atomic"
	"time"
)

type pingCallbackFn func(pong *proto.Pong)

type swimServer struct {
	logger   *zap.Logger
	localIPs iputil.IPList
	opts     *Options
	ascon    core.ASCON

	hostAddress proto.IP
	incarnation uint16

	udpControl    wire.UDPControlServer
	tcpControl    wire.TCPControlServer
	nodes         core.NodeManager
	gossip        core.GossipManager
	multicastComm wire.MulticastCommunicator
	loop          core.RunLoop
	syncManager   core.SyncManager

	bootstrapResponseReceived atomic.Bool
	bootstrapResponse         chan *proto.Packet
	bootstrapCompleted        chan bool

	indirectPingCallbacks containers.SyncMap[uint32, pingCallbackFn]

	lastFullSyncNode proto.NodeHash
}

func newSwimServer(bindAddress netip.Addr, allIPs iputil.IPList, opts *Options) (*swimServer, error) {
	ipClass := "v4"
	if bindAddress.Is6() {
		ipClass = "v6"
	}
	logger := opts.LogHandler.With(zap.String("facility", "swim-"+ipClass))

	ascon, err := core.NewASCON(opts.CryptoKey)
	if err != nil {
		return nil, fmt.Errorf("failed initializing crypto handler: %w", err)
	}

	selfAddress := proto.IPFromAddr(bindAddress)

	srv := &swimServer{
		logger:      logger,
		localIPs:    allIPs,
		nodes:       core.NewNodeManager(opts.LogHandler, selfAddress),
		gossip:      core.NewGossipManager(opts.LogHandler, 1),
		opts:        opts,
		ascon:       ascon,
		hostAddress: selfAddress,

		bootstrapResponse:  make(chan *proto.Packet, 32),
		bootstrapCompleted: make(chan bool, 2),
	}

	var (
		multicastAddr     string
		multicastAddrKind proto.AddressKind
	)
	if bindAddress.Is6() {
		multicastAddr = opts.IPv6MulticastAddress
		multicastAddrKind = proto.AddressIPv6
	} else {
		multicastAddr = opts.IPv4MulticastAddress
		multicastAddrKind = proto.AddressIPv4
	}

	multicastComm, err := wire.NewMulticastCommunicator(opts.LogHandler, multicastAddr, opts.MulticastPort, multicastAddrKind, srv)
	if err != nil {
		return nil, fmt.Errorf("failed initializing multicast handler: %w", err)
	}
	srv.multicastComm = multicastComm

	udpControl, err := wire.NewUDPControlServer(opts.LogHandler, srv.hostAddress, opts.SWIMPort, srv)
	if err != nil {
		return nil, err
	}
	srv.udpControl = udpControl

	tcpControl, err := wire.NewTCPControlServer(opts.LogHandler, srv.hostAddress, opts.SWIMPort, srv)
	if err != nil {
		return nil, err
	}
	srv.tcpControl = tcpControl

	syncManager := core.NewSyncManager(opts.LogHandler, srv)
	srv.syncManager = syncManager

	srv.loop = core.NewRunLoop(opts.LogHandler, opts.ProtocolPeriod, srv, opts.UDPMaxLength, opts.UseAdaptivePingTimeout, opts.FullSyncProtocolRounds)
	return srv, nil
}

func (s *swimServer) Start() {
	go s.multicastComm.Listen()
	go s.udpControl.Listen()
	s.tcpControl.Start()
	s.loop.Start()
	go s.runBootstrap()
}

func (s *swimServer) Shutdown() {
	s.tcpControl.Shutdown()
	s.multicastComm.Shutdown()
	s.udpControl.Shutdown()
	s.loop.Shutdown()
}

// ---------------------------- SyncManagerDelegate ----------------------------

func (s *swimServer) BootstrapCompleted(core.SyncManager) {
	s.bootstrapCompleted <- true
}

func (s *swimServer) HandleIncomingNodeList(_ core.SyncManager, nodes []proto.Node) {
	s.nodes.Bootstrap(s.loop.CurrentPeriod(), nodes)
}

func (s *swimServer) DispatchTCPPacket(_ core.SyncManager, msg proto.Message, client wire.TCPControlClient) error {
	data, err := proto.EncTCPPkt(s.incarnation, msg, s.ascon.Seal)
	if err != nil {
		return err
	}
	return client.Write(data)
}

func (s *swimServer) EstablishTCP(_ core.SyncManager, node proto.Node) (wire.TCPControlClient, error) {
	return s.tcpControl.Dial(node.Address.Bytes(), s.opts.SWIMPort)
}

func (s *swimServer) AllKnownNodes(core.SyncManager) []proto.Node {
	allNodes := s.nodes.All(core.NodeTypeStable | core.NodeTypeRemote)
	return append(allNodes, proto.Node{
		Suspect:     false,
		Incarnation: s.incarnation,
		Address:     s.hostAddress,
	})
}

func (s *swimServer) PerformBootstrapComplete(_ core.SyncManager, target proto.Node) {
	s.nodes.Add(s.loop.CurrentPeriod(), &target)
	s.logger.Debug("Completed bootstrapping new member", zap.String("target", target.Address.String()))
	s.gossip.Add(&proto.Event{
		Payload: &proto.Join{
			Source:      s.hostAddress,
			Subject:     target.Address,
			Incarnation: target.Incarnation,
		},
	})
}

// -------------------------- TCPPacketEncoder/Decoder -------------------------

func (s *swimServer) DecodeTCPPacket(tcpPkt *proto.TCPPacket, client wire.TCPControlClient) (*proto.Packet, error) {
	data := s.ascon.Open(tcpPkt.Payload)
	if data == nil {
		return nil, nil
	}
	source := client.Source()

	pkt, err := proto.ParsePacket(data)
	if err != nil {
		return nil, err
	}
	pkt.Source = source.IP
	return pkt, nil
}

func (s *swimServer) EncodeTCPPacket(message proto.Message) (*proto.TCPPacket, error) {
	return proto.EncTCPPkt(s.incarnation, message, s.ascon.Seal)
}

// ------------------------- TCPControlServerDelegate --------------------------

func (s *swimServer) HandleTCPPacket(server wire.TCPControlServer, client wire.TCPControlClient, tcpPkt *proto.TCPPacket) {
	pkt, err := s.DecodeTCPPacket(tcpPkt, client)
	if err != nil {
		s.logger.Error("Failed decoding TCP packet",
			zap.String("source", client.Source().String()),
			zap.Error(err))
		client.Close()
		return
	}

	sync, ok := pkt.Message.(*proto.Sync)
	if !ok {
		s.logger.Warn("Received unexpected packet over TCP",
			zap.String("type", pkt.Header.OpCode.String()),
			zap.String("source", client.Source().String()))
		client.Close()
		return
	}

	switch sync.Mode {
	case proto.SyncModeBootstrap:
		s.syncManager.HandleBootstrap(sync, client)

	case proto.SyncModeFirstPhase:
		s.syncManager.HandleSyncRequest(sync, client)

	case proto.SyncModeSecondPhase:
		s.logger.Warn("Received out-of-order sync packet",
			zap.String("source", client.Source().String()))
		client.Close()

	case proto.SyncModeBootstrapAck:
		s.logger.Warn("Received stray bootstrap ack message",
			zap.String("source", client.Source().String()))
	}
}

// ------------------------- UDPControlServerDelegate --------------------------

func (s *swimServer) HandleUDPControlMessage(server wire.UDPControlServer, data []byte, source *net.UDPAddr) {
	data = s.ascon.Open(data)
	if data == nil {
		s.logger.Warn("Failed decrypting UDP control traffic", zap.String("source", source.String()))
		return
	}
	pkt, err := proto.ParsePacket(data)
	if err != nil {
		s.logger.Error("Failed parsing UDP control packet",
			zap.String("source", source.String()),
			zap.Error(err))
		return
	}
	pkt.Source = source.IP

	switch msg := pkt.Message.(type) {
	case *proto.BootstrapResponse:
		if s.bootstrapResponseReceived.Swap(true) {
			return // We are already bootstrapping with another peer. Ignore.
		}
		s.bootstrapResponse <- pkt

	case *proto.Ping:
		// Immediately return a pong
		lastKnown := s.gossip.LastGossipAbout(proto.IPFromNetIP(pkt.Source))
		lastInc := uint16(0)
		var lastEventKind proto.EventKind
		if lastKnown != nil {
			lastInc = lastKnown.Payload.GetIncarnation()
			lastEventKind = lastKnown.EventKind()
		}

		s.DispatchMessage(pkt.Source, proto.Pong{
			Ack:                  msg.TargetIncarnation == s.incarnation,
			IsLastStateKnown:     lastKnown != nil,
			Cookie:               msg.Cookie,
			Period:               msg.Period,
			LastKnownIncarnation: lastInc,
			LastKnownEventKind:   lastEventKind,
		})
		node := &proto.Node{
			Suspect:     false,
			Incarnation: pkt.Header.Incarnation,
			Address:     proto.IPFromNetIP(pkt.Source),
		}

		s.logger.Debug("Ping received",
			zap.String("source", pkt.Source.String()),
			logutil.StringerArr("events", msg.Events))

		s.nodes.Add(s.loop.CurrentPeriod(), node)
		s.nodes.MarkStable(node.Hash())

		s.processPingEvents(msg.Events)

	case *proto.Pong:
		s.logger.Debug("Pong received", zap.String("source", pkt.Source.String()))

		s.logger.Debug("Received pong data",
			zap.Bool("has_last_state", msg.IsLastStateKnown),
			zap.String("last_ev", msg.LastKnownEventKind.String()),
			zap.Uint16("last_inc", msg.LastKnownIncarnation))

		if ev := msg.LastKnownEventKind; msg.IsLastStateKnown &&
			((ev == proto.EventFaulty || ev == proto.EventSuspect) && msg.LastKnownIncarnation >= s.incarnation) ||
			msg.LastKnownIncarnation > s.incarnation {
			s.incrementIncarnationToSurpass(msg.LastKnownIncarnation)
		}

		indirectTag := uint32(msg.Cookie) | uint32(msg.Period)<<16
		if callback, ok := s.indirectPingCallbacks.LoadAndDelete(indirectTag); ok {
			callback(msg)
		}
		s.loop.ReceiveMessage(pkt)
	case *proto.IndirectPong:
		s.loop.ReceiveMessage(pkt)

	case *proto.IndirectPing:
		go s.performIndirectPing(msg, pkt)

	case *proto.BootstrapAck:
		s.syncManager.PerformBootstrap(proto.Node{
			Suspect:     false,
			Incarnation: pkt.Header.Incarnation,
			Address:     proto.IPFromNetIP(source.IP),
		})

	default:
		s.logger.Warn("Received (and ignored) unexpected message",
			zap.String("opcode", pkt.Header.OpCode.String()),
			zap.String("source", pkt.Source.String()))
	}
}

func (s *swimServer) isEventLocal(event proto.Event) bool {
	return event.Payload.GetSubject().Equal(&s.hostAddress)
}

func (s *swimServer) processPingEvents(events []proto.Event) {
	lastIncarnation := -1
	period := s.loop.CurrentPeriod()
	reannounce := false
	for _, e := range events {
		func(ev proto.Event) {
			s.gossip.Add(&ev)
		}(e)

		if s.isEventLocal(e) {
			if ev := e.EventKind(); ev == proto.EventSuspect || ev == proto.EventFaulty {
				remoteIncarnation := e.Payload.GetIncarnation()
				reannounce = reannounce || remoteIncarnation < s.incarnation
				if remoteIncarnation >= s.incarnation {
					lastIncarnation = int(e.Payload.GetIncarnation())
				}
			} else if inc := e.Payload.GetIncarnation(); inc > s.incarnation {
				lastIncarnation = int(inc)
			}
			if lastIncarnation > -1 {
				s.logger.Info("Updating incarnation in response of event",
					zap.Uint16("current", s.incarnation),
					zap.Int("received", lastIncarnation),
					zap.String("event", e.String()))
			}
			continue
		}

		switch msg := e.Payload.(type) {
		case *proto.Faulty:
			n := s.nodes.NodeByIP(msg.Subject)
			if n == nil || n.Incarnation > msg.Incarnation {
				break // switch
			}
			s.nodes.MarkFaulty(n.Hash())

		case *proto.Suspect:
			s.nodes.UpdateOrCreate(period, &proto.Node{
				Suspect:     true,
				Incarnation: msg.Incarnation,
				Address:     msg.Subject,
			})

		case *proto.Join:
			s.nodes.UpdateOrCreate(period, &proto.Node{
				Suspect:     false,
				Incarnation: msg.Incarnation,
				Address:     msg.Subject,
			})
		case *proto.Healthy:
			s.nodes.UpdateOrCreate(period, &proto.Node{
				Suspect:     false,
				Incarnation: msg.Incarnation,
				Address:     msg.Subject,
			})

		case *proto.Left:
			hash := msg.Subject.IntoHash()
			node := s.nodes.NodeByHash(hash)
			if node == nil || node.Incarnation > msg.Incarnation {
				continue
			}
			s.nodes.Delete(hash)

		case *proto.Alive:
			s.nodes.UpdateOrCreate(period, &proto.Node{
				Suspect:     false,
				Incarnation: msg.Incarnation,
				Address:     msg.Subject,
			})

		case *proto.Wrap:
			s.nodes.WrapIncarnation(period, &proto.Node{
				Suspect:     false,
				Incarnation: msg.Incarnation,
				Address:     msg.Subject,
			})
		}
	}

	// Check if we need to re-announce ourselves
	if lastIncarnation > -1 {
		s.incrementIncarnationToSurpass(uint16(lastIncarnation))
		return
	}

	if reannounce {
		s.gossip.Add(&proto.Event{
			Payload: &proto.Alive{
				Subject:     s.hostAddress,
				Incarnation: s.incarnation,
			},
		})
	}
}

func (s *swimServer) incrementIncarnationToSurpass(lastKnown uint16) {
	if lastKnown+1 > math.MaxUint16 {
		// Incrementing would overflow our counter. Reset it, and announce
		// a wrap event.
		s.incarnation = 0
		s.gossip.Add(&proto.Event{
			Payload: &proto.Wrap{
				Subject:        s.hostAddress,
				Incarnation:    lastKnown,
				NewIncarnation: 0,
			},
		})
	} else {
		s.incarnation = lastKnown + 1
		s.gossip.Add(&proto.Event{
			Payload: &proto.Alive{
				Subject:     s.hostAddress,
				Incarnation: s.incarnation,
			},
		})
	}
}

func (s *swimServer) performIndirectPing(msg *proto.IndirectPing, pkt *proto.Packet) {
	cookie := uint16(rand.Int())
	period := s.loop.CurrentPeriod()
	tag := uint32(cookie) | uint32(period)<<16
	s.indirectPingCallbacks.Store(tag, func(pong *proto.Pong) {
		s.DispatchMessage(pkt.Source, &proto.IndirectPong{
			Cookie: msg.Cookie,
			Period: msg.Period,
		})
	})

	s.DispatchMessage(msg.Address.Bytes(), &proto.Ping{
		Cookie:            cookie,
		Period:            period,
		TargetIncarnation: 0,
		Events:            nil,
	})

	go func() {
		<-time.After(s.opts.ProtocolPeriod * 5)
		s.indirectPingCallbacks.Delete(tag)
	}()
}

// ------------------------- MulticastListenerDelegate -------------------------

func (s *swimServer) LocalIPs() *iputil.IPList { return &s.localIPs }

func (s *swimServer) HandleMulticastMessage(listener wire.MulticastCommunicator, data []byte, source net.IP) {
	data = s.ascon.Open(data)
	if data == nil {
		s.logger.Debug("ASCON open failed", zap.String("source", source.String()))
		return
	}

	pkt, err := proto.ParsePacket(data)
	if err != nil {
		s.logger.Error("Failed parsing multicast packet",
			zap.String("source", source.String()),
			zap.Error(err))
		return
	}

	pkt.Source = source

	if pkt.Header.OpCode != proto.BTRP {
		s.logger.Debug("Ignoring multicast with incorrect OPCode", zap.String("received", pkt.Header.OpCode.String()), zap.String("source", source.String()))
		return // Nothing to do here.
	}

	// Should we answer this bootstrap request?
	if rand.Intn(2) == 0 {
		s.logger.Debug("Ignoring bootstrap request due to bad luck", zap.String("source", source.String()))
		return // Nope.
	}

	// Try to answer it. We will get an ack over the control channel in case
	// the node that sent the request wants to continue bootstrapping with this
	// instance.
	s.DispatchMessage(source, proto.BootstrapResponse{})

	s.logger.Debug("Pushing BootstrapResponse", zap.String("target", source.String()))
}

func (s *swimServer) DispatchMulticast(message proto.Message) {
	buf := proto.EncPkt(s.incarnation, message)
	buf, err := s.ascon.Seal(buf)
	if err != nil {
		s.logger.Error("CRITICAL: Could not seal outgoing message", zap.Error(err))
		return
	}

	if err = s.multicastComm.Write(buf); err != nil {
		s.logger.Error("Failed writing multicast", zap.Error(err))
	}
}

// ------------------------------ RunLoopDelegate ------------------------------

func (s *swimServer) NextProbeSubject() *proto.Node { return s.nodes.NextPing() }

func (s *swimServer) GossipEvents(maxLen int) []proto.Event { return s.gossip.Next(maxLen) }

func (s *swimServer) DispatchMessage(dst net.IP, message proto.Message) {
	buf := proto.EncPkt(s.incarnation, message)
	buf, err := s.ascon.Seal(buf)
	if err != nil {
		s.logger.Error("CRITICAL: Could not seal outgoing message", zap.Error(err))
		return
	}

	rawRAddr := netip.AddrPortFrom(iputil.ConvertNetIP(dst), s.opts.SWIMPort)

	rAddr := net.UDPAddrFromAddrPort(rawRAddr)
	n, err := s.udpControl.WriteUDP(rAddr, buf)
	if err != nil {
		s.logger.Error("Failed writing UDP", zap.String("target", dst.String()), zap.Error(err))
		return
	}
	if n < len(buf) {
		s.logger.Error("CRITICAL: Short UDP write", zap.String("target", dst.String()), zap.Int("msg_len", len(buf)), zap.Int("sent", n))
		return
	}
}

func (s *swimServer) SignalHealthyNode(hash proto.NodeHash) {
	s.logger.Debug("Node is healthy", zap.String("node", hash.String()))
	s.nodes.MarkStable(hash)
}

func (s *swimServer) SignalSuspectNode(hash proto.NodeHash) {
	s.logger.Debug("Node is suspect", zap.String("node", hash.String()))
	s.nodes.MarkSuspect(s.loop.CurrentPeriod(), false, hash)
	node := s.nodes.NodeByHash(hash)
	if node == nil {
		return
	}

	s.gossip.Add(&proto.Event{
		Payload: &proto.Suspect{
			Source:      s.hostAddress,
			Subject:     node.Address,
			Incarnation: node.Incarnation,
		},
	})
}

func (s *swimServer) ProcessExpiredSuspects() {
	period := s.loop.CurrentPeriod()
	for _, n := range s.nodes.ExpiredSuspects(period, s.opts.SuspectPeriods) {
		s.logger.Debug("Node is faulty", zap.String("node", n.String()))
		s.nodes.MarkFaulty(n.Hash())
		s.gossip.Add(&proto.Event{
			Payload: &proto.Faulty{
				Source:      s.hostAddress,
				Subject:     n.Address,
				Incarnation: n.Incarnation,
			},
		})
	}
}

func (s *swimServer) PingTimeout() time.Duration { return s.opts.PingTimeout }

func (s *swimServer) IndirectPingMembers(node proto.NodeHash) []*proto.Node {
	return s.nodes.IndirectPingCandidates(s.opts.IndirectPings, node)
}

func (s *swimServer) NodeByHash(hash proto.NodeHash) *proto.Node {
	return s.nodes.NodeByHash(hash)
}

func (s *swimServer) PerformFullSync() {
	foundFirst := false
	var firstNode proto.Node
	anySelected := false
	var selected proto.Node
	s.nodes.Range(core.NodeTypeStable, func(n *proto.Node) bool {
		if !foundFirst {
			foundFirst = true
			firstNode = *n
		}

		if n.Hash() != s.lastFullSyncNode {
			selected = *n
			anySelected = true
			return false
		}

		return true
	})

	// If we have no node to use, bail
	if !anySelected && !foundFirst {
		return
	}

	// If we found no node other than the last we synced with, use it anyway.
	if !anySelected {
		selected = firstNode
	}
	s.lastFullSyncNode = selected.Hash()
	s.syncManager.PerformSync(selected)
}

// --------------------------- Bootstrap Facilities ----------------------------

func (s *swimServer) runBootstrap() {
start:
	// drain the channel
	containers.DrainChan(s.bootstrapResponse)

	s.bootstrapResponseReceived.Store(false)
	publishTimer := time.NewTicker(2 * time.Second)
	var bootstrapResponse *proto.Packet
loop:
	for {
		select {
		case <-publishTimer.C:
			s.DispatchMulticast(proto.BootstrapRequest{})
		case bootstrapResponse = <-s.bootstrapResponse:
			if bootstrapResponse.Header.OpCode != proto.BTRR {
				break // select
			}
			s.bootstrapResponseReceived.Store(true)
			s.logger.Debug("Received bootstrap response")
			publishTimer.Stop()
			break loop
		}
	}

	containers.DrainChan(s.bootstrapResponse)

	s.DispatchMessage(bootstrapResponse.Source, proto.BootstrapAck{})
	s.syncManager.SetBootstrapSource(bootstrapResponse.Source)

	bootstrapTimeout := time.NewTimer(5 * time.Second)
	select {
	case <-s.bootstrapCompleted:
	case <-bootstrapTimeout.C:
		s.logger.Debug("Reached bootstrap completion timeout. Restarting advertise")
		s.syncManager.SetBootstrapSource(net.IP{})
		goto start
	}

	s.logger.Info("Bootstrap completed.")
	containers.DrainChan(s.bootstrapCompleted)
}
