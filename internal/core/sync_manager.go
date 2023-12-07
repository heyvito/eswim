package core

import (
	"github.com/heyvito/eswim/internal/logutil"
	"github.com/heyvito/eswim/internal/proto"
	"github.com/heyvito/eswim/internal/wire"
	"go.uber.org/zap"
	"net"
	"time"
)

type SyncManager interface {
	HandleBootstrap(msg *proto.Sync, client wire.TCPControlClient)
	HandleSyncRequest(msg *proto.Sync, client wire.TCPControlClient)
	PerformSync(target proto.Node)
	PerformBootstrap(target proto.Node)
	SetBootstrapSource(source net.IP)
}

type SyncManagerDelegate interface {
	TCPPacketDecoder
	TCPPacketEncoder
	BootstrapCompleted(manager SyncManager)
	HandleIncomingNodeList(manager SyncManager, nodes []proto.Node)
	DispatchTCPPacket(manager SyncManager, msg proto.Message, client wire.TCPControlClient) error
	EstablishTCP(manager SyncManager, node proto.Node) (wire.TCPControlClient, error)
	AllKnownNodes(manager SyncManager) []proto.Node
	PerformBootstrapComplete(manager SyncManager, target proto.Node)
}

func NewSyncManager(logger *zap.Logger, delegate SyncManagerDelegate) SyncManager {
	return &syncManager{
		logger:              logger.With(zap.String("facility", "sync-manager")),
		delegate:            delegate,
		acceptBootstrapFrom: nil,
	}
}

type syncManager struct {
	logger   *zap.Logger
	delegate SyncManagerDelegate

	acceptBootstrapFrom net.IP
}

func (s *syncManager) SetBootstrapSource(source net.IP) {
	s.logger.Debug("Now accepting bootstrap from remote host", zap.String("source", source.String()))
	s.acceptBootstrapFrom = source
}

func (s *syncManager) PerformBootstrap(target proto.Node) {
	go func() {
		client, err := s.delegate.EstablishTCP(s, target)
		if err != nil {
			s.logger.Error("Failed establishing TCP connection for bootstrap",
				zap.String("target", target.Address.String()),
				zap.Error(err))
			return
		}
		respChan := client.Hijack()
		defer client.Close()

		pkt, err := s.delegate.EncodeTCPPacket(proto.Sync{
			Nodes: s.delegate.AllKnownNodes(s),
			Mode:  proto.SyncModeBootstrap,
		})
		if err != nil {
			s.logger.Error("Delegate failed to encode TCP packet",
				zap.Error(err))
			return
		}
		if err = client.Write(pkt); err != nil {
			s.logger.Error("Failed writing data to client", zap.Error(err))
			return
		}

		timeout := time.NewTimer(1 * time.Minute)
		select {
		case pkt = <-respChan:
			timeout.Stop()
			decoded, err := s.delegate.DecodeTCPPacket(pkt, client)
			if err != nil {
				s.logger.Error("Failed decoding bootstrap response from client",
					zap.String("client", client.Source().String()),
					zap.Error(err))
				return
			}
			if decoded == nil {
				s.logger.Error("Failed decoding bootstrap response from client. Decode generated no output.",
					zap.String("client", client.Source().String()))
				return
			}

			resp, ok := decoded.Message.(*proto.Sync)
			if !ok {
				s.logger.Warn("Client bootstrap response was not the expected type",
					zap.String("received", decoded.Header.OpCode.String()),
					zap.String("client", client.Source().String()))
				return
			}
			if resp.Mode != proto.SyncModeBootstrapAck {
				s.logger.Warn("Client bootstrap response had wrong mode",
					zap.String("received", resp.Mode.String()),
					zap.String("client", client.Source().String()))
				return
			}

			s.logger.Debug("Finished bootstrapping client",
				zap.String("client", client.Source().String()))

			s.delegate.PerformBootstrapComplete(s, target)

		case <-timeout.C:
			s.logger.Warn("Timeout reached waiting for client bootstrap response",
				zap.String("client", client.Source().String()))
		}
	}()
}

func (s *syncManager) HandleSyncRequest(msg *proto.Sync, client wire.TCPControlClient) {
	go func() {
		defer client.Close()
		s.delegate.HandleIncomingNodeList(s, msg.Nodes)
		encoded, err := s.delegate.EncodeTCPPacket(&proto.Sync{
			Nodes: s.delegate.AllKnownNodes(s),
			Mode:  proto.SyncModeSecondPhase,
		})
		if err != nil {
			s.logger.Error("Delegate failed to encode sync response",
				zap.Error(err),
				zap.String("client", client.Source().String()))
			return
		}
		if err = client.Write(encoded); err != nil {
			s.logger.Error("Failed writing sync response",
				zap.String("client", client.Source().String()),
				zap.Error(err))
		}
	}()
}

func (s *syncManager) PerformSync(target proto.Node) {
	go func() {
		client, err := s.delegate.EstablishTCP(s, target)
		if err != nil {
			s.logger.Error("Failed establishing TCP connection for bootstrap",
				zap.String("target", target.Address.String()),
				zap.Error(err))
			return
		}
		respChan := client.Hijack()
		defer client.Close()

		pkt, err := s.delegate.EncodeTCPPacket(proto.Sync{
			Nodes: s.delegate.AllKnownNodes(s),
			Mode:  proto.SyncModeFirstPhase,
		})
		if err != nil {
			s.logger.Error("Delegate failed to encode TCP packet",
				zap.Error(err))
			return
		}
		if err = client.Write(pkt); err != nil {
			s.logger.Error("Failed writing data to client", zap.Error(err))
			return
		}

		timeout := time.NewTimer(1 * time.Minute)
		select {
		case pkt = <-respChan:
			timeout.Stop()
			decoded, err := s.delegate.DecodeTCPPacket(pkt, client)
			if err != nil {
				s.logger.Error("Failed decoding sync response from client",
					zap.String("client", client.Source().String()),
					zap.Error(err))
				return
			}
			if decoded == nil {
				s.logger.Error("Failed decoding sync response from client. Decode generated no output.",
					zap.String("client", client.Source().String()))
				return
			}

			resp, ok := decoded.Message.(*proto.Sync)
			if !ok {
				s.logger.Warn("Client sync response was not the expected type",
					zap.String("received", decoded.Header.OpCode.String()),
					zap.String("client", client.Source().String()))
				return
			}
			if resp.Mode != proto.SyncModeSecondPhase {
				s.logger.Warn("Client sync response had wrong mode",
					zap.String("received", resp.Mode.String()),
					zap.String("client", client.Source().String()))
				return
			}

			s.logger.Debug("Finished full-sync with client",
				zap.String("client", client.Source().String()))

			s.delegate.HandleIncomingNodeList(s, resp.Nodes)

		case <-timeout.C:
			s.logger.Warn("Timeout reached waiting for client sync response",
				zap.String("client", client.Source().String()))
		}
	}()
}

func (s *syncManager) HandleBootstrap(msg *proto.Sync, client wire.TCPControlClient) {
	defer client.Close()
	source := client.Source()

	if !source.IP.Equal(s.acceptBootstrapFrom) {
		s.logger.Warn("Received bootstrap attempt from unexpected source",
			zap.String("destination", source.String()))
		return
	}

	s.acceptBootstrapFrom = net.IP{}
	s.logger.Debug("Notifying delegate of received node list", logutil.StringerArr("list", msg.Nodes))
	s.delegate.HandleIncomingNodeList(s, msg.Nodes)
	s.logger.Debug("Notifying delegate of completed bootstrap process", zap.String("source", source.String()))
	s.delegate.BootstrapCompleted(s)

	// Ack and Close
	syncAck := proto.Sync{Mode: proto.SyncModeBootstrapAck}
	err := s.delegate.DispatchTCPPacket(s, syncAck, client)
	if err != nil {
		s.logger.Error("Failed writing TCP packet",
			zap.String("destination", source.String()),
			zap.Error(err))
	}
}
