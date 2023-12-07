package wire

import (
	"context"
	"errors"
	"fmt"
	"github.com/heyvito/eswim/internal/iputil"
	"github.com/heyvito/eswim/internal/proto"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// MulticastCommunicatorDelegate implements a component capable of responding to
// requests from the MulticastCommunicator.
type MulticastCommunicatorDelegate interface {
	// LocalIPs is responsible for returning a list of all local IP addresses.
	LocalIPs() *iputil.IPList

	// HandleMulticastMessage is responsible for interpreting and handling a
	// given message received from the Multicast network.
	HandleMulticastMessage(listener MulticastCommunicator, data []byte, source net.IP)
}

// MulticastCommunicator abstracts a common set of methods implemented by components
// responsible for reading and writing UDP messages over the Multicast network.
type MulticastCommunicator interface {
	// Listen starts the listener loop responsible for reading messages from the
	// multicast network and sending data read to the provided delegate. This
	// method will block the current calling routine until Shutdown is called.
	Listen()

	// Write writes a given buffer to the Multicast network.
	Write(data []byte) error

	// Shutdown starts the shutdown process for the current
	// MulticastCommunicator. It is safe to call this method multiple times.
	Shutdown()
}

// NewMulticastCommunicator returns a new MulticastCommunicator
func NewMulticastCommunicator(logger *zap.Logger, multicastAddress string, multicastPort uint16, ipProto proto.AddressKind, delegate MulticastCommunicatorDelegate) (MulticastCommunicator, error) {
	var bindAddress string
	var network string
	if ipProto == proto.AddressIPv4 {
		bindAddress = fmt.Sprintf("%s:%d", multicastAddress, multicastPort)
		network = "udp4"

	} else {
		bindAddress = fmt.Sprintf("[%s]:%d", multicastAddress, multicastPort)
		network = "udp6"
	}

	log := logger.With(
		zap.String("facility", "multicast_listener"),
		zap.String("network", network),
		zap.String("address", multicastAddress))

	udpAddr, err := net.ResolveUDPAddr(network, bindAddress)
	if err != nil {
		log.Error("Error resolving UDP address",
			zap.Error(err))
		return nil, err
	}

	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var opErr error
			err = c.Control(func(fd uintptr) {
				opErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
			})

			if err != nil {
				log.Error("Error applying SO_REUSEPORT port to connection",
					zap.Error(err))
				return err
			}
			return opErr
		},
	}

	m := &multicastCommunicator{
		udpMulticastAddress: udpAddr,
		network:             network,
		log:                 log,
		listenConfig:        lc,
		address:             multicastAddress,
		port:                multicastPort,
		bindAddress:         bindAddress,
		ipProto:             ipProto,
		stopping:            &atomic.Bool{},
		delegate:            delegate,
	}

	return m, nil
}

type multicastCommunicator struct {
	udpMulticastAddress *net.UDPAddr
	network             string
	log                 *zap.Logger
	listenConfig        net.ListenConfig
	address             string
	bindAddress         string
	ipProto             proto.AddressKind
	stopping            *atomic.Bool
	port                uint16
	delegate            MulticastCommunicatorDelegate
	listenerMu          sync.Mutex
	currentListener     *net.UDPConn
}

func (m *multicastCommunicator) Listen() {
	buffer := make([]byte, 4094)
	backoff := 2 * time.Second

	for {
		if m.stopping.Load() {
			break
		}
		listener, err := m.makeListener()
		if err != nil {
			m.log.Error("Failed creating listener",
				zap.Duration("backoff", backoff),
				zap.Error(err))
			time.Sleep(backoff)
			continue
		}

		m.listenerMu.Lock()
		m.currentListener = listener
		m.listenerMu.Unlock()

		m.log.Info("Initialized listener")
		for {
			n, addr, err := listener.ReadFromUDP(buffer)
			if err != nil {
				if m.stopping.Load() {
					break
				}

				m.log.Error("Error reading UDP", zap.Error(err))
				continue
			}
			ip, _ := netip.AddrFromSlice(addr.IP)
			if m.delegate.LocalIPs().Contains(ip) {
				continue
			}

			m.delegate.HandleMulticastMessage(m, buffer[:n], addr.IP)
		}

		stopping := m.stopping.Load()
		if err = listener.Close(); err != nil && !stopping {
			m.log.Error("Failed closing listener. Check for leaked descriptors.", zap.Error(err))
		}
		if stopping {
			m.log.Info("Stopped listener")
			break
		}
	}
}

func (m *multicastCommunicator) makeListener() (*net.UDPConn, error) {
	lp, err := m.listenConfig.ListenPacket(context.Background(), m.network, m.bindAddress)
	if err != nil {
		m.log.Error("Error initializing packet listener", zap.Error(err))
		return nil, err
	}

	conn := lp.(*net.UDPConn)
	sconn, err := conn.SyscallConn()
	if err != nil {
		m.log.Error("Error creating raw network connection", zap.Error(err))
		return nil, err
	}

	var setErr error
	err = sconn.Control(func(fd uintptr) {
		ipProto := m.ipProto.UnixProto()
		if ipProto == unix.IPPROTO_IPV6 {
			multicastIPComponents := []uint8(net.ParseIP(m.address).To16())

			mreq := unix.IPv6Mreq{
				Multiaddr: [16]byte(multicastIPComponents),
				Interface: 0,
			}
			setErr = unix.SetsockoptIPv6Mreq(int(fd), ipProto, unix.IPV6_JOIN_GROUP, &mreq)
		} else {
			multicastIPComponents := []uint8(net.ParseIP(m.address).To4())

			mreq := unix.IPMreq{
				Multiaddr: [4]byte(multicastIPComponents),
				Interface: [4]byte{0, 0, 0, 0},
			}

			setErr = unix.SetsockoptIPMreq(int(fd), ipProto, unix.IP_ADD_MEMBERSHIP, &mreq)
		}
	})

	if err != nil {
		m.log.Error("Error joining multicast group", zap.Error(err))
		return nil, err
	}

	if setErr != nil {
		return nil, setErr
	}

	return conn, nil
}

func (m *multicastCommunicator) Write(data []byte) error {
	conn, err := net.DialUDP(m.network, nil, m.udpMulticastAddress)
	if err != nil {
		m.log.Error("Error dialing network", zap.Error(err))
		return err
	}

	toWrite := len(data)
	written := 0
	for written < toWrite {
		n, err := conn.Write(data[written:])
		if err != nil {
			m.log.Error("Error writing message", zap.Error(err))
			_ = conn.Close()
			return err
		}
		written += n
	}

	return conn.Close()
}

func (m *multicastCommunicator) Shutdown() {
	if m.stopping.Swap(true) {
		return
	}
	m.listenerMu.Lock()
	defer m.listenerMu.Unlock()

	if m.currentListener != nil {
		if err := m.currentListener.Close(); err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			m.log.Error("Failed closing current listener", zap.Error(err))
		}
	}
}
