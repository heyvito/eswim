package eswim

import (
	"fmt"
	"github.com/heyvito/eswim/internal/core"
	"github.com/heyvito/eswim/internal/iputil"
	"github.com/heyvito/eswim/internal/proto"
	"go.uber.org/zap"
	"net/http"
	"strings"
)

type Server struct {
	opts               *Options
	v4Server, v6Server *swimServer
}

func NewServer(opts *Options) (*Server, error) {
	var err error
	if err = opts.normalize(); err != nil {
		return nil, err
	}

	logger := opts.LogHandler.With(zap.String("facility", "server"))

	addrs := opts.HostAddresses
	if addrs.IPv6Address == nil {
		logger.Warn("IPv6 is set to autoconfigure, but no usable address could be found. Disabling.")
		addrs.IPv6Address = IgnoreFamilyAddr
	} else if addrs.IPv6Address == nil && opts.IPv6MulticastAddress != "" {
		logger.Warn("IPv6 is set to autoconfigure, but no usable address could be found. Disabling.")
		addrs.IPv6Address = IgnoreFamilyAddr
	} else if addrs.IPv6Address != nil && opts.IPv6MulticastAddress == "" {
		logger.Warn("IPv6 is either set to autoconfigure or has been provided manually, but IPv6MulticastAddress is empty. Disabling.")
		addrs.IPv6Address = IgnoreFamilyAddr
	}

	if addrs.IPv4Address == nil {
		logger.Warn("IPv4 is set to autoconfigure, but no usable address could be found. Disabling.")
		addrs.IPv4Address = IgnoreFamilyAddr
	} else if addrs.IPv4Address == nil && opts.IPv4MulticastAddress != "" {
		logger.Warn("IPv4 is set to autoconfigure, but no usable address could be found. Disabling.")
		addrs.IPv4Address = IgnoreFamilyAddr
	} else if addrs.IPv4Address != nil && opts.IPv4MulticastAddress == "" {
		logger.Warn("IPv4 is either set to autoconfigure or has been provided manually, but IPv4MulticastAddress is empty. Disabling.")
		addrs.IPv4Address = IgnoreFamilyAddr
	}

	allIPs, err := iputil.LocalAddresses()
	if err != nil {
		return nil, err
	}

	var v4Server, v6Server *swimServer
	if addrs.IPv4Address != IgnoreFamilyAddr {
		v4Server, err = newSwimServer(*addrs.IPv4Address, allIPs, opts)
	}
	if err != nil {
		return nil, fmt.Errorf("failed initializing IPv4 server: %w", err)
	}

	if addrs.IPv6Address != IgnoreFamilyAddr {
		v6Server, err = newSwimServer(*addrs.IPv6Address, allIPs, opts)
	}
	if err != nil {
		return nil, fmt.Errorf("failed initializing IPv6 server: %w", err)
	}

	return &Server{opts, v4Server, v6Server}, nil
}

func (s *Server) Start() {
	if s.v4Server != nil {
		s.v4Server.Start()
	}
	if s.v6Server != nil {
		s.v6Server.Start()
	}

	go func() {
		http.ListenAndServe(":2727", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			str := strings.Builder{}
			dumpServerState("IPv4", &str, s.v4Server)
			dumpServerState("IPv6", &str, s.v6Server)
			w.Header().Add("Content-Type", "text/plain")
			_, _ = w.Write([]byte(str.String()))
		}))
	}()
}

func dumpServerState(name string, str *strings.Builder, server *swimServer) {
	if server == nil {
		return
	}

	str.WriteString(name + "\n")
	str.WriteString("==========================\n")
	str.WriteString(fmt.Sprintf("Address: %s\n", server.hostAddress.String()))
	str.WriteString(fmt.Sprintf("Period: %d\n", server.loop.CurrentPeriod()))
	str.WriteString(fmt.Sprintf("Incarnation: %d\n\n", server.incarnation))

	str.WriteString("Known Nodes\n")
	server.nodes.Range(core.NodeTypeInvalid, func(n *proto.Node) bool {
		str.WriteString("  - " + n.String() + "\n")
		return true
	})

	str.WriteString("\nGossips\n")
	for _, e := range server.gossip.CurrentGossips() {
		str.WriteString("  - " + e.String() + "\n")
	}
}

func (s *Server) Shutdown() {
	if s.v4Server != nil {
		s.v4Server.Shutdown()
	}
	if s.v6Server != nil {
		s.v6Server.Shutdown()
	}
}
