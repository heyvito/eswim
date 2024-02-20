package eswim

import (
	"fmt"
	"github.com/heyvito/eswim/internal/core"
	"github.com/heyvito/eswim/internal/iputil"
	"net/netip"
)

type Server interface {
	Start()
	Shutdown()
	GetKnownNodes() []netip.Addr
}

type server struct {
	opts               *Options
	v4Server, v6Server *swimServer
	stateServer        *stateServer
}

func NewServer(opts *Options) (Server, error) {
	var err error
	if err = opts.normalize(); err != nil {
		return nil, err
	}

	logger := opts.LogHandler.Named("server")

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

	srv := &server{opts: opts, v4Server: v4Server, v6Server: v6Server}
	if opts.StateServerPort > 0 {
		var stateServer *stateServer
		stateServer, err = newStateServer(logger, srv, opts.StateServerPort)
		if err != nil {
			return nil, fmt.Errorf("failed initializing state server: %w", err)
		}
		srv.stateServer = stateServer
	}

	return srv, nil
}

func (s *server) Start() {
	if s.v4Server != nil {
		s.v4Server.Start()
	}
	if s.v6Server != nil {
		s.v6Server.Start()
	}
	if s.stateServer != nil {
		s.stateServer.start()
	}
}

func (s *server) Shutdown() {
	if s.stateServer != nil {
		s.stateServer.stop()
	}
	if s.v4Server != nil {
		s.v4Server.Shutdown()
	}
	if s.v6Server != nil {
		s.v6Server.Shutdown()
	}
}

func (s *server) GetKnownNodes() []netip.Addr {
	var list []netip.Addr
	filter := core.NodeTypeStable | core.NodeTypeStable
	if s.v4Server != nil {
		for _, v := range s.v4Server.nodes.All(filter) {
			list = append(list, v.Address.IntoManaged())
		}
	}
	if s.v6Server != nil {
		for _, v := range s.v6Server.nodes.All(filter) {
			list = append(list, v.Address.IntoManaged())
		}
	}

	return list
}
