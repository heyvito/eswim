package eswim

import (
	"context"
	"errors"
	"fmt"
	"github.com/heyvito/eswim/internal/core"
	"github.com/heyvito/eswim/internal/proto"
	"github.com/heyvito/eswim/resources"
	"go.uber.org/zap"
	"html/template"
	"io"
	"net"
	"net/http"
	"strings"
)

type statePage struct {
	HostIPs string
	Servers []stateServerObj
}
type stateServerObj struct {
	ServerIP    string
	TCPPort     int
	UDPPort     int
	Incarnation int
	Period      int
	Members     []stateMember
	Gossips     []stateGossip
}
type stateMember struct {
	IP          string
	Status      string
	Incarnation int
}
type stateGossip struct {
	Source      string
	Kind        string
	Subject     string
	Incarnation int
}

type stateServer struct {
	server         *server
	httpServer     *http.Server
	l              net.Listener
	headTemplate   *template.Template
	serverTemplate *template.Template
	footerTemplate *template.Template
	logger         *zap.Logger
}

func newStateServer(logger *zap.Logger, parent *server, port int) (*stateServer, error) {
	head, err := template.New("head").Parse(resources.StatePageHead)
	if err != nil {
		return nil, err
	}

	srv, err := template.New("server").Parse(resources.StatePageServer)
	if err != nil {
		return nil, err
	}

	footer, err := template.New("footer").Parse(resources.StatePageFooter)
	if err != nil {
		return nil, err
	}

	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}

	return &stateServer{
		logger:         logger,
		server:         parent,
		l:              l,
		headTemplate:   head,
		serverTemplate: srv,
		footerTemplate: footer,
	}, nil
}

func (s *stateServer) render(output io.Writer) error {
	var servers []stateServerObj
	var ips []string

	if s.server.v4Server != nil {
		servers = append(servers, s.processServer(s.server.v4Server))
		ips = append(ips, s.server.v4Server.hostAddress.String())
	}

	if s.server.v6Server != nil {
		servers = append(servers, s.processServer(s.server.v6Server))
		ips = append(ips, s.server.v6Server.hostAddress.String())
	}

	page := statePage{
		HostIPs: strings.Join(ips, ", "),
		Servers: servers,
	}

	if err := s.headTemplate.Execute(output, page); err != nil {
		return err
	}

	for _, srv := range page.Servers {
		if err := s.serverTemplate.Execute(output, srv); err != nil {
			return err
		}
	}

	if err := s.footerTemplate.Execute(output, page); err != nil {
		return err
	}

	return nil
}

func (s *stateServer) start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		err := s.render(writer)
		if err != nil {
			writer.WriteHeader(500)
			_, _ = writer.Write([]byte("Internal server error"))
			s.logger.Error("Failed serving state request", zap.Error(err))
		}
	})
	s.httpServer = &http.Server{
		Handler: mux,
	}

	go func() {
		if err := s.httpServer.Serve(s.l); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("State Server failed", zap.Error(err))
		}
	}()
}

func (s *stateServer) processServer(srv *swimServer) stateServerObj {
	var members []stateMember
	var gossips []stateGossip

	for _, m := range srv.nodes.All(core.NodeTypeAll) {
		state := "stable"
		if m.Suspect {
			state = "suspect"
		}
		members = append(members, stateMember{
			IP:          m.Address.String(),
			Status:      state,
			Incarnation: int(m.Incarnation),
		})
	}

	for _, g := range srv.gossip.CurrentGossips() {
		source := ""
		if ev, ok := g.Payload.(proto.EventWithSource); ok {
			source = ev.GetSource().String()
		}

		gossips = append(gossips, stateGossip{
			Source:      source,
			Kind:        strings.ToLower(strings.TrimPrefix(g.EventKind().String(), "Event")),
			Subject:     g.Payload.GetSubject().String(),
			Incarnation: int(g.Payload.GetIncarnation()),
		})
	}

	return stateServerObj{
		ServerIP:    srv.hostAddress.String(),
		TCPPort:     srv.tcpControl.Address().Port,
		UDPPort:     srv.udpControl.Address().Port,
		Incarnation: int(srv.incarnation),
		Period:      int(srv.loop.CurrentPeriod()),
		Members:     members,
		Gossips:     gossips,
	}
}

func (s *stateServer) stop() {
	if s.httpServer != nil {
		_ = s.httpServer.Shutdown(context.Background())
	}
}
