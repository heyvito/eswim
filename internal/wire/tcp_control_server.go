package wire

import (
	"errors"
	"fmt"
	"github.com/heyvito/eswim/internal/proto"
	"go.uber.org/zap"
	"net"
	"sync"
	"sync/atomic"
)

// TCPControlServer represents handler for TCP communications
type TCPControlServer interface {
	// Start starts the server in a new routine.
	Start()

	// Shutdown signals the server to shut down and blocks the calling routine
	// until all clients disconnect.
	Shutdown()

	// Dial opens a new socket to a remote control server specified by addr
	Dial(addr proto.IP, port uint16) (TCPControlClient, error)
}

// TCPControlServerDelegate implements methods that allow handling of incoming
// data from a TCPControlClient accepted by a provided TCPControlServer.
type TCPControlServerDelegate interface {
	HandleTCPPacket(server TCPControlServer, client TCPControlClient, tcpPkt *proto.TCPPacket)
}

type tcpControlServer struct {
	log      *zap.Logger
	listener *net.TCPListener
	address  *net.TCPAddr
	delegate TCPControlServerDelegate
	running  atomic.Bool
	clients  sync.WaitGroup
	network  string
}

func (t *tcpControlServer) Dial(addr proto.IP, port uint16) (TCPControlClient, error) {
	var strAddr string
	if addr.AddressKind == proto.AddressIPv4 {
		strAddr = fmt.Sprintf("%s:%d", addr, port)
	} else {
		strAddr = fmt.Sprintf("[%s]:%d", addr, port)
	}
	conn, err := net.Dial(t.network, strAddr)
	if err != nil {
		return nil, err
	}
	return t.makeClient(conn), nil
}

func (t *tcpControlServer) Start() {
	if t.running.Swap(true) {
		return
	}
	go t.loop()
}

func (t *tcpControlServer) Shutdown() {
	if !t.running.Swap(false) {
		return
	}
	if err := t.listener.Close(); err != nil {
		if errors.Is(err, net.ErrClosed) {
			return
		}
	}

	// Wait for drain
	t.clients.Wait()
}

func (t *tcpControlServer) makeClient(conn net.Conn) TCPControlClient {
	t.clients.Add(1)
	client := &tcpControlClient{
		log:      t.log.With(zap.String("client", conn.RemoteAddr().String())),
		conn:     conn,
		done:     t.clients.Done,
		closed:   make(chan bool),
		message:  make(chan *proto.TCPPacket, 16),
		decoder:  proto.TCPPacketDecoder.New(),
		isClosed: atomic.Bool{},
	}
	client.onMessage = func(pkt *proto.TCPPacket) {
		t.delegate.HandleTCPPacket(t, client, pkt)
	}
	go client.serve()
	return client
}

func (t *tcpControlServer) loop() {
	t.log.Info("Now listening for TCP control messages", zap.String("address", t.address.String()))
	for t.running.Load() {
		conn, err := t.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				continue
			}
			t.log.Error("Failed accepting client", zap.Error(err))
		}
		t.makeClient(conn)
	}
}

// NewTCPControlServer returns a new TCPControlServer
func NewTCPControlServer(log *zap.Logger, address proto.IP, port uint16, delegate TCPControlServerDelegate) (TCPControlServer, error) {
	var network string
	var addrPort string
	if address.AddressKind == proto.AddressIPv6 {
		network = "tcp6"
		addrPort = fmt.Sprintf("[%s]:%d", address.String(), port)
	} else {
		network = "tcp4"
		addrPort = fmt.Sprintf("%s:%d", address.String(), port)
	}

	addr, err := net.ResolveTCPAddr(network, addrPort)
	if err != nil {
		return nil, fmt.Errorf("failed resolving TCP address: %w", err)
	}

	listener, err := net.ListenTCP(network, addr)
	if err != nil {
		return nil, fmt.Errorf("failed initializing TCP listener: %w", err)
	}

	return &tcpControlServer{
		log:      log.With(zap.String("facility", "tcp_control_server"), zap.String("network", network)),
		network:  network,
		listener: listener,
		address:  addr,
		delegate: delegate,
	}, nil
}
