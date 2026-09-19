package ble

import (
	"sync"

	"github.com/EdmundLimBoEn/stage-wand/host/internal/protocol"
)

// Hooks are the host session and input injector. BLE code calls these and
// does not import the WebSocket server.
type Hooks struct {
	Code      func() string
	SetPeer   func(string)
	OnCommand func(protocol.Command)
}

// Action is work the GATT adapter applies after a write, kick, or disconnect.
type Action struct {
	Notify    []byte
	Broadcast bool
	Drop      string
}

// Session is the Apple BluetoothServer policy: the host is the GATT
// peripheral, one authed central, JSON Command/Reply on the frozen UUIDs.
type Session struct {
	hooks  Hooks
	mu     sync.Mutex
	authed string
}

func NewSession(hooks Hooks) *Session {
	if hooks.Code == nil {
		hooks.Code = func() string { return "" }
	}
	if hooks.SetPeer == nil {
		hooks.SetPeer = func(string) {}
	}
	if hooks.OnCommand == nil {
		hooks.OnCommand = func(protocol.Command) {}
	}
	return &Session{hooks: hooks}
}

func (s *Session) Handle(client string, payload []byte) []Action {
	if client == "" {
		client = "unknown"
	}
	command, err := protocol.ParseCommand(payload)
	s.mu.Lock()
	authed := s.authed
	s.mu.Unlock()
	if authed != client {
		if err != nil {
			return nil
		}
		auth, ok := command.(protocol.Auth)
		if !ok {
			return nil
		}
		if auth.Code != s.hooks.Code() {
			if authed == "" {
				return []Action{{Notify: mustReply(protocol.Bye{Reason: protocol.ReasonBadAuth}), Broadcast: true}}
			}
			return []Action{{Drop: client}}
		}
		var actions []Action
		if authed != "" && authed != client {
			actions = append(actions, Action{Drop: authed})
		}
		s.mu.Lock()
		s.authed = client
		s.mu.Unlock()
		s.hooks.SetPeer("Bluetooth")
		return append(actions, Action{Notify: mustReply(protocol.Status{})})
	}
	if err != nil {
		return nil
	}
	switch v := command.(type) {
	case protocol.Auth:
		if v.Code == s.hooks.Code() {
			return []Action{{Notify: mustReply(protocol.Status{})}}
		}
		return nil
	case protocol.Move:
		if !protocol.MoveInRange(v.Dx, v.Dy) {
			return nil
		}
		s.hooks.OnCommand(v)
	case protocol.Scroll:
		s.hooks.OnCommand(v)
	default:
		s.hooks.OnCommand(command)
	}
	return nil
}

func (s *Session) Kick() []Action {
	s.mu.Lock()
	authed := s.authed
	s.authed = ""
	s.mu.Unlock()
	if authed == "" {
		return nil
	}
	s.hooks.SetPeer("")
	return []Action{{Notify: mustReply(protocol.Bye{Reason: protocol.ReasonKicked}), Broadcast: true}}
}

func (s *Session) Drop(client string) {
	s.mu.Lock()
	if s.authed != client {
		s.mu.Unlock()
		return
	}
	s.authed = ""
	s.mu.Unlock()
	s.hooks.SetPeer("")
}

func (s *Session) Authed() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.authed
}

func mustReply(reply protocol.Reply) []byte {
	data, err := protocol.EncodeReply(reply)
	if err != nil {
		return nil
	}
	return data
}
