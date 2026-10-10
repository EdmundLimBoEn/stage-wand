package server

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/EdmundLimBoEn/stage-wand/host/internal/control"
	"github.com/EdmundLimBoEn/stage-wand/host/internal/protocol"
	"github.com/gorilla/websocket"
)

type Config struct {
	Ports      []int
	AuthWait   time.Duration
	PingEvery  time.Duration
	PongWait   time.Duration
	MaxMessage int64
}

func DefaultConfig() Config {
	return Config{
		Ports:      []int{8787, 8788, 8789, 8790},
		AuthWait:   2 * time.Second,
		PingEvery:  3 * time.Second,
		PongWait:   6 * time.Second,
		MaxMessage: 16384,
	}
}

type Session = control.Session

func NewSession(code string) *Session { return control.NewSession(code) }

type Server struct {
	cfg       Config
	session   *Session
	onCommand func(protocol.Command)
	upgrader  websocket.Upgrader
	mu        sync.Mutex
	peers     map[*peer]struct{}
	http      *http.Server
	listener  net.Listener
	moves     int
	closed    bool
}

type peer struct {
	conn     *websocket.Conn
	remote   string
	mu       sync.Mutex
	authed   bool
	owner    *control.Owner
	lastPong time.Time
	closed   chan struct{}
	once     sync.Once
}

func New(session *Session, cfg Config, onCommand func(protocol.Command)) *Server {
	if cfg.AuthWait == 0 {
		cfg.AuthWait = 2 * time.Second
	}
	if cfg.PingEvery == 0 {
		cfg.PingEvery = 3 * time.Second
	}
	if cfg.PongWait == 0 {
		cfg.PongWait = 6 * time.Second
	}
	if cfg.MaxMessage == 0 {
		cfg.MaxMessage = 16384
	}
	if len(cfg.Ports) == 0 {
		cfg.Ports = []int{8787, 8788, 8789, 8790}
	}
	if onCommand == nil {
		onCommand = func(protocol.Command) {}
	}
	return &Server{
		cfg:       cfg,
		session:   session,
		onCommand: onCommand,
		peers:     map[*peer]struct{}{},
		upgrader: websocket.Upgrader{
			EnableCompression: false,
			HandshakeTimeout:  5 * time.Second,
		},
	}
}

func (s *Server) Start() (int, error) {
	var last error
	for _, port := range s.cfg.Ports {
		ln, err := net.Listen("tcp", net.JoinHostPort("0.0.0.0", strconv.Itoa(port)))
		if err != nil {
			last = err
			continue
		}
		actual := ln.Addr().(*net.TCPAddr).Port
		s.listener = ln
		s.session.SetPort(actual)
		mux := http.NewServeMux()
		mux.HandleFunc("/", s.handle)
		s.http = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
		go s.http.Serve(ln)
		return actual, nil
	}
	if last == nil {
		last = errNoPort
	}
	return 0, last
}

var errNoPort = errString("no Stage Wand port available in 8787-8790")

type errString string

func (e errString) Error() string { return string(e) }

func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	var err error
	if s.http != nil {
		err = s.http.Close()
	}
	s.Kick()
	return err
}

func (s *Server) Kick() {
	s.mu.Lock()
	peers := make([]*peer, 0, len(s.peers))
	for p := range s.peers {
		peers = append(peers, p)
	}
	s.mu.Unlock()
	for _, p := range peers {
		s.closePeer(p, protocol.ReasonKicked)
	}
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	unavailable := s.closed || len(s.peers) >= 64
	s.mu.Unlock()
	if unavailable {
		http.Error(w, "Stage Wand is busy", http.StatusServiceUnavailable)
		return
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	if tcp, ok := conn.UnderlyingConn().(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
	}
	conn.SetReadLimit(s.cfg.MaxMessage)
	p := &peer{
		conn:     conn,
		remote:   r.RemoteAddr,
		lastPong: time.Now(),
		closed:   make(chan struct{}),
	}
	p.owner = &control.Owner{Name: p.remote, Revoke: func(reason string) { s.closePeer(p, reason) }}
	conn.SetPongHandler(func(string) error {
		p.mu.Lock()
		p.lastPong = time.Now()
		p.mu.Unlock()
		return nil
	})
	s.mu.Lock()
	if s.closed || len(s.peers) >= 64 {
		s.mu.Unlock()
		conn.Close()
		return
	}
	s.peers[p] = struct{}{}
	s.mu.Unlock()
	go s.read(p)
}

func (s *Server) read(p *peer) {
	defer s.session.Release(p.owner)
	defer s.closePeer(p, "")
	timer := time.AfterFunc(s.cfg.AuthWait, func() {
		p.mu.Lock()
		authed := p.authed
		p.mu.Unlock()
		if !authed {
			s.closePeer(p, "")
		}
	})
	defer timer.Stop()
	for {
		_, data, err := p.conn.ReadMessage()
		if err != nil {
			return
		}
		command, err := protocol.ParseCommand(data)
		p.mu.Lock()
		authed := p.authed
		p.mu.Unlock()
		if !authed {
			if err != nil {
				return
			}
			auth, ok := command.(protocol.Auth)
			if !ok {
				return
			}
			ok, revoke := s.session.Claim(auth.Code, p.owner)
			if !ok {
				s.closePeer(p, protocol.ReasonBadAuth)
				return
			}
			p.mu.Lock()
			p.authed = true
			p.lastPong = time.Now()
			p.mu.Unlock()
			timer.Stop()
			revoke()
			if err := p.write(protocol.Status{}); err != nil {
				return
			}
			go s.heartbeat(p)
			continue
		}
		if err != nil {
			continue
		}
		switch v := command.(type) {
		case protocol.Auth:
			accepted, revoke := s.session.Claim(v.Code, p.owner)
			if !accepted {
				s.closePeer(p, protocol.ReasonBadAuth)
				return
			}
			revoke()
			if err := p.write(protocol.Status{}); err != nil {
				return
			}
			continue
		case protocol.Move:
			if !protocol.MoveInRange(v.Dx, v.Dy) {
				continue
			}
			s.mu.Lock()
			s.moves++
			count := s.moves
			s.mu.Unlock()
			if count%200 == 0 {
				fmt.Printf("MOVES %d\n", count)
				_ = os.Stdout.Sync()
			}
			s.session.Dispatch(p.owner, func() { s.onCommand(v) })
		case protocol.Scroll:
			if !protocol.MoveInRange(v.Dx, v.Dy) {
				continue
			}
			s.session.Dispatch(p.owner, func() { s.onCommand(v) })
		default:
			s.session.Dispatch(p.owner, func() { s.onCommand(command) })
		}
	}
}

func (s *Server) heartbeat(p *peer) {
	ticker := time.NewTicker(s.cfg.PingEvery)
	defer ticker.Stop()
	for {
		select {
		case <-p.closed:
			return
		case <-ticker.C:
			p.mu.Lock()
			last := p.lastPong
			p.mu.Unlock()
			if time.Since(last) >= s.cfg.PongWait {
				s.closePeer(p, "")
				return
			}
			deadline := time.Now().Add(time.Second)
			p.mu.Lock()
			err := p.conn.WriteControl(websocket.PingMessage, nil, deadline)
			p.mu.Unlock()
			if err != nil {
				s.closePeer(p, "")
				return
			}
		}
	}
}

func (s *Server) closePeer(p *peer, reason string) {
	p.once.Do(func() {
		if reason != "" {
			_ = p.write(protocol.Bye{Reason: reason})
		}
		p.mu.Lock()
		deadline := time.Now().Add(time.Second)
		_ = p.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), deadline)
		p.conn.Close()
		p.mu.Unlock()
		close(p.closed)
		s.mu.Lock()
		delete(s.peers, p)
		s.mu.Unlock()
		s.session.Release(p.owner)
	})
}

func (p *peer) write(reply protocol.Reply) error {
	data, err := protocol.EncodeReply(reply)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	return p.conn.WriteMessage(websocket.TextMessage, data)
}
