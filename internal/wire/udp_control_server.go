package wire

import (
	"errors"
	"fmt"
	"github.com/heyvito/eswim/internal/proto"
	"go.uber.org/zap"
	"net"
	"sync/atomic"
	"time"
)

// UDPControlServerDelegate implements methods that allow handling of incoming
// data from a UDPControlServer.
type UDPControlServerDelegate interface {
	HandleUDPControlMessage(server UDPControlServer, data []byte, source *net.UDPAddr)
}

// UDPControlServer represents a component responsible for reading incoming UDP
// requests from other nodes.
type UDPControlServer interface {
	// Listen starts the server on the calling routine, blocking until it is
	// shutdown.
	Listen()

	// Shutdown signals the server to shut down, unblocking the routine that '
	// invoked Listen
	Shutdown()

	// WriteUDP writes a given data buffer to the specified address and port.
	// Returns the amount of bytes written, or an error
	WriteUDP(target *net.UDPAddr, data []byte) (int, error)
}

// NewUDPControlServer returns a new UDPControlServer
func NewUDPControlServer(logger *zap.Logger, address proto.IP, swimPort uint16, delegate UDPControlServerDelegate) (UDPControlServer, error) {
	var network string
	var addrPort string
	if address.AddressKind == proto.AddressIPv6 {
		network = "udp6"
		addrPort = fmt.Sprintf("[%s]:%d", address.String(), swimPort)
	} else {
		network = "udp4"
		addrPort = fmt.Sprintf("%s:%d", address.String(), swimPort)
	}

	addr, err := net.ResolveUDPAddr(network, addrPort)
	if err != nil {
		return nil, fmt.Errorf("failed resolving %s address: %w", network, err)
	}

	return &udpControlServer{
		log:      logger.With(zap.String("facility", "udp_control_server"), zap.String("network", network)),
		network:  network,
		address:  addr,
		delegate: delegate,
	}, nil
}

type udpControlServer struct {
	log      *zap.Logger
	address  *net.UDPAddr
	listener *net.UDPConn
	stopping atomic.Bool
	delegate UDPControlServerDelegate
	network  string
}

func (u *udpControlServer) WriteUDP(target *net.UDPAddr, data []byte) (int, error) {
	return u.listener.WriteToUDP(data, target)
}

func (u *udpControlServer) Listen() {
	for !u.stopping.Load() {
		if !u.makeListener() {
			continue
		}
		u.readLoop()
	}
}

func (u *udpControlServer) makeListener() bool {
	backoff := 2 * time.Second
	listener, err := net.ListenUDP(u.network, u.address)
	if err != nil {
		u.log.Error("Failed initializing control listener",
			zap.Duration("backoff", backoff),
			zap.Error(err))
		time.Sleep(backoff)
		return false
	}

	u.listener = listener
	u.log.Info("Now listening for UDP control messages",
		zap.String("address", u.address.String()))
	return true
}

func (u *udpControlServer) readLoop() {
	buf := make([]byte, 4096)
	for !u.stopping.Load() {
		n, addr, err := u.listener.ReadFromUDP(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				break
			}

			u.log.Error("Error reading message", zap.Error(err))
			continue
		}

		u.delegate.HandleUDPControlMessage(u, buf[:n], addr)
	}
}

func (u *udpControlServer) Shutdown() {
	if u.stopping.Swap(true) {
		return
	}

	if u.listener == nil {
		return
	}

	if err := u.listener.Close(); err != nil {
		if !errors.Is(err, net.ErrClosed) {
			u.log.Error("Failed closing UDP control listener", zap.Error(err))
		}
	}
}
