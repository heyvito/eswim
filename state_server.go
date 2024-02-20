package eswim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/heyvito/eswim/internal/core"
	"github.com/heyvito/eswim/internal/proto"
	"github.com/heyvito/eswim/resources"
	"go.uber.org/zap"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

var sharedStateServer *stateServer = nil

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type statePage struct {
	HostIPs string
	Servers []stateServerObj
}

type stateServerObj struct {
	ServerIP    string        `json:"serverIP,omitempty"`
	IPClass     string        `json:"ipClass,omitempty"`
	TCPPort     int           `json:"tcpPort,omitempty"`
	UDPPort     int           `json:"udpPort,omitempty"`
	Incarnation int           `json:"incarnation,omitempty"`
	Period      int           `json:"period,omitempty"`
	Members     []stateMember `json:"members,omitempty"`
	Gossips     []stateGossip `json:"gossips,omitempty"`
}

type stateMember struct {
	IP          string `json:"ip,omitempty"`
	Status      string `json:"status,omitempty"`
	Incarnation int    `json:"incarnation,omitempty"`
}

type stateGossip struct {
	// Only available for gossip notifications:

	Direction string `json:"direction,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`

	// Always available:

	Source      string `json:"source,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Subject     string `json:"subject,omitempty"`
	Incarnation int    `json:"incarnation,omitempty"`
}

func stateGossipFromEvent(g proto.Event) stateGossip {
	source := ""
	if ev, ok := g.Payload.(proto.EventWithSource); ok {
		source = ev.GetSource().String()
	}

	return stateGossip{
		Source:      source,
		Kind:        strings.ToLower(strings.TrimPrefix(g.EventKind().String(), "Event")),
		Subject:     g.Payload.GetSubject().String(),
		Incarnation: int(g.Payload.GetIncarnation()),
	}
}

type stateUpdate struct {
	// Used exclusively by gossips
	ts        time.Time
	direction string

	Kind       string `json:"kind,omitempty"`
	ServerKind string `json:"serverKind,omitempty"`
	Payload    any    `json:"payload,omitempty"`
}

type stateHubClient struct {
	send chan []byte
	hub  *stateHub
	conn *websocket.Conn
}

func (c *stateHubClient) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			if _, err = w.Write(message); err != nil {
				return
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *stateHubClient) readPump() {
	defer func() {
		c.hub.unregister <- c
		_ = c.conn.Close()
	}()
	c.conn.SetReadLimit(512)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
	}
}

type stateHub struct {
	clients    map[*stateHubClient]bool
	broadcast  chan any
	register   chan *stateHubClient
	unregister chan *stateHubClient

	hasClient bool
	logger    *zap.Logger
}

func (s *stateHub) stop() {
	for c := range s.clients {
		s.unregister <- c
	}
}

func (s *stateHub) run() {
	for {
		select {
		case client := <-s.register:
			s.clients[client] = true
			s.hasClient = true
		case client := <-s.unregister:
			if _, ok := s.clients[client]; ok {
				delete(s.clients, client)
				close(client.send)
			}
			s.hasClient = len(s.clients) > 0

		case object := <-s.broadcast:
			v, err := json.Marshal(object)
			if err != nil {
				s.logger.Error("Failed marshalling object", zap.Error(err))
				continue
			}
			for client := range s.clients {
				select {
				case client.send <- v:
				default:
					close(client.send)
					delete(s.clients, client)
					s.hasClient = len(s.clients) > 0
				}
			}
		}
	}
}

type stateServer struct {
	server         *server
	httpServer     *http.Server
	l              net.Listener
	headTemplate   *template.Template
	serverTemplate *template.Template
	footerTemplate *template.Template
	logger         *zap.Logger
	hub            *stateHub
	gossipIntake   chan stateUpdate
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

	sharedStateServer = &stateServer{
		logger:         logger,
		server:         parent,
		l:              l,
		headTemplate:   head,
		serverTemplate: srv,
		footerTemplate: footer,
		gossipIntake:   make(chan stateUpdate, 128),
		hub: &stateHub{
			clients:    map[*stateHubClient]bool{},
			broadcast:  make(chan any, 256),
			register:   make(chan *stateHubClient, 256),
			unregister: make(chan *stateHubClient, 256),
			hasClient:  false,
			logger:     logger.Named("hub"),
		},
	}

	return sharedStateServer, nil
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

	if _, err := output.Write([]byte("<script type=\"text/javascript\">\n" + resources.StatePageScript + "\n</script>")); err != nil {
		return err
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
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			s.logger.Error("Failed upgrading connection", zap.Error(err))
			return
		}

		s.serviceClient(conn)
	})

	s.httpServer = &http.Server{
		Handler: mux,
	}

	go func() {
		if err := s.httpServer.Serve(s.l); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("State Server failed", zap.Error(err))
		}
	}()

	go s.hub.run()

	go func() {
		t := time.NewTicker(5 * time.Second)
		for {
			select {
			case <-t.C:
				if !s.hub.hasClient {
					continue
				}
				if s.server.v4Server != nil {
					srv := s.processServer(s.server.v4Server)
					s.hub.broadcast <- stateUpdate{
						Kind:       "general",
						ServerKind: "v4",
						Payload:    srv,
					}
				}

				if s.server.v6Server != nil {
					srv := s.processServer(s.server.v6Server)
					s.hub.broadcast <- stateUpdate{
						Kind:       "general",
						ServerKind: "v6",
						Payload:    srv,
					}
				}
			case ev := <-s.gossipIntake:
				if !s.hub.hasClient {
					continue
				}
				convertedGossip := stateGossipFromEvent(*ev.Payload.(*proto.Event))
				convertedGossip.Direction = ev.direction
				convertedGossip.Timestamp = ev.ts.Format("02-Jan 15:04:05")
				ev.Payload = convertedGossip
				s.hub.broadcast <- ev
			}
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
		gossips = append(gossips, stateGossipFromEvent(g))
	}

	return stateServerObj{
		IPClass:     srv.ipClass,
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
		s.hub.stop()
		_ = s.httpServer.Shutdown(context.Background())
	}
}

func (s *stateServer) serviceClient(conn *websocket.Conn) {
	c := &stateHubClient{
		send: make(chan []byte, 256),
		conn: conn,
		hub:  s.hub,
	}
	s.hub.register <- c

	go c.writePump()
	go c.readPump()
}

func (s *stateServer) registerGossip(direction, serverKind string, event *proto.Event) {
	if s == nil {
		return
	}
	s.gossipIntake <- stateUpdate{
		ts:        time.Now(),
		direction: direction,

		Kind:       "gossip",
		ServerKind: serverKind,
		Payload:    event,
	}
}
