package server

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

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

type Session struct {
	mu   sync.Mutex
	code string
	peer string
	port int
}

func NewSession(code string) *Session {
	return &Session{code: code}
}

func (s *Session) Code() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.code
}

func (s *Session) SetCode(code string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.code = code
}

func (s *Session) Peer() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.peer
}

func (s *Session) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.port
}

func (s *Session) setPeer(peer string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peer = peer
}

func (s *Session) setPort(port int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.port = port
}

type Server struct {
	cfg       Config
	session   *Session
	onCommand func(protocol.Command)
	upgrader  websocket.Upgrader
	mu        sync.Mutex
	peers     map[*peer]struct{}
	active    *peer
	http      *http.Server
	listener  net.Listener
	moves     int
}

type peer struct {
	conn     *websocket.Conn
	remote   string
	mu       sync.Mutex
	authed   bool
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
			CheckOrigin:       func(*http.Request) bool { return true },
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
		s.session.setPort(actual)
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
	s.Kick()
	s.mu.Lock()
	for p := range s.peers {
		p.conn.Close()
	}
	s.mu.Unlock()
	var err error
	if s.http != nil {
		err = s.http.Close()
	}
	return err
}

func (s *Server) Kick() {
	s.mu.Lock()
	active := s.active
	s.mu.Unlock()
	if active != nil {
		s.closePeer(active, protocol.ReasonKicked)
	}
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
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
	conn.SetPongHandler(func(string) error {
		p.mu.Lock()
		p.lastPong = time.Now()
		p.mu.Unlock()
		return nil
	})
	s.mu.Lock()
	s.peers[p] = struct{}{}
	s.mu.Unlock()
	go s.read(p)
}

func (s *Server) read(p *peer) {
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
			if auth.Code != s.session.Code() {
				s.closePeer(p, protocol.ReasonBadAuth)
				return
			}
			s.displace(p)
			p.mu.Lock()
			p.authed = true
			p.lastPong = time.Now()
			p.mu.Unlock()
			timer.Stop()
			s.session.setPeer(p.remote)
			if err := p.write(protocol.Status{}); err != nil {
				return
			}
			go s.heartbeat(p)
			continue
		}
		if err != nil {
			continue
		}
		s.mu.Lock()
		active := s.active == p
		s.mu.Unlock()
		if !active {
			continue
		}
		switch v := command.(type) {
		case protocol.Auth:
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
			s.onCommand(v)
		case protocol.Scroll:
			s.onCommand(v)
		default:
			s.onCommand(command)
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

func (s *Server) displace(p *peer) {
	s.mu.Lock()
	old := s.active
	s.active = p
	s.moves = 0
	s.mu.Unlock()
	if old != nil && old != p {
		s.closePeer(old, protocol.ReasonDisplaced)
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
		if s.active == p {
			s.active = nil
			s.session.setPeer("")
		}
		s.mu.Unlock()
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
