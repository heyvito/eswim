package wire

import (
	"errors"
	"github.com/heyvito/eswim/internal/fsm"
	"github.com/heyvito/eswim/internal/proto"
	"go.uber.org/zap"
	"net"
	"sync"
	"sync/atomic"
)

// TCPControlClient represents a client connected to a TCPControlServer
type TCPControlClient interface {
	// Write writes the provided proto.TCPPacket to the client's socket
	Write(data *proto.TCPPacket) error

	// Close closes the client socket and stops all associated routines
	Close()

	// Source returns the TCP Address of the remote client
	Source() *net.TCPAddr

	// Hijack prevents incoming messages to be delivered to the default handler,
	// those being delivered instead directly to the channel returned by this
	// method.
	Hijack() chan *proto.TCPPacket
}

type tcpControlClient struct {
	log       *zap.Logger
	conn      net.Conn
	done      func()
	onMessage func(packet *proto.TCPPacket)
	closed    chan bool
	message   chan *proto.TCPPacket
	decoder   fsm.ExportedFSM[proto.TCPPacket]
	isClosed  atomic.Bool

	hijackMu   sync.Mutex
	isHijacked bool
	hijackChan chan *proto.TCPPacket
}

func (c *tcpControlClient) Hijack() chan *proto.TCPPacket {
	c.hijackMu.Lock()
	defer c.hijackMu.Unlock()

	if c.isHijacked {
		panic("double-hijack on TCPControlClient")
	}

	c.isHijacked = true
	c.hijackChan = make(chan *proto.TCPPacket, 10)
	return c.hijackChan
}

func (c *tcpControlClient) Write(pkt *proto.TCPPacket) error {
	size := pkt.RequiredSize()
	buf := make([]byte, size)
	pkt.Encode(buf)

	written := 0
	for written < size {
		n, err := c.conn.Write(buf[written:])
		if err != nil {
			return err
		}
		written += n
	}
	return nil
}

func (c *tcpControlClient) Source() *net.TCPAddr {
	return c.conn.RemoteAddr().(*net.TCPAddr)
}

func (c *tcpControlClient) Close() {
	if c.isClosed.Swap(true) {
		return
	}

	defer close(c.closed)
	if err := c.conn.Close(); err != nil {
		if errors.Is(err, net.ErrClosed) {
			return
		}
		c.log.Error("Error closing client", zap.Error(err))
	}
}

func (c *tcpControlClient) loop() {
	buf := make([]byte, 1024)
	for {
		n, err := c.conn.Read(buf)
		if err != nil {
			if c.isClosed.Load() && errors.Is(err, net.ErrClosed) {
				// We are closing the socket, but was still waiting to read
				// data. Just ignore it and carry on.
				return
			}

			c.log.Error("Failed reading", zap.Error(err))
			c.Close()
			return
		}
		for _, v := range buf[:n] {
			msg, err := c.decoder.Feed(v)
			if err != nil {
				c.log.Error("Failed decoding message from client", zap.Error(err))
				c.Close()
				return
			}
			if msg != nil {
				c.hijackMu.Lock()
				if c.isHijacked {
					c.hijackChan <- msg
				} else {
					c.message <- msg
				}
				c.hijackMu.Unlock()
			}
		}
	}
}

func (c *tcpControlClient) serve() {
	defer c.done()
	go c.loop()

	for {
		select {
		case <-c.closed:
			return
		case msg := <-c.message:
			c.onMessage(msg)
		}
	}
}
