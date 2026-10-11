package ble

import (
	"sync"

	"github.com/EdmundLimBoEn/stage-wand/host/internal/control"
	"github.com/EdmundLimBoEn/stage-wand/host/internal/protocol"
)

type Hooks struct {
	Control   *control.Session
	OnCommand func(protocol.Command)
	OnActions func([]Action)
}

type Action struct {
	Notify []byte
	Client string
	Drop   string
	Reason string
}

type Session struct {
	hooks      Hooks
	mu         sync.Mutex
	operations sync.Mutex
	authed     string
	owner      *control.Owner
	closed     bool
}

func NewSession(hooks Hooks) *Session {
	if hooks.Control == nil {
		panic("BLE requires a shared control session")
	}
	if hooks.OnCommand == nil {
		hooks.OnCommand = func(protocol.Command) {}
	}
	if hooks.OnActions == nil {
		hooks.OnActions = func([]Action) {}
	}
	return &Session{hooks: hooks}
}

func (s *Session) Handle(client string, payload []byte) []Action {
	s.operations.Lock()
	if s.closed {
		s.operations.Unlock()
		return nil
	}
	actions, revoke := s.handle(client, payload)
	if len(actions) > 0 {
		s.hooks.OnActions(actions)
	}
	s.operations.Unlock()
	if revoke != nil {
		revoke()
	}
	return actions
}

func (s *Session) handle(client string, payload []byte) ([]Action, func()) {
	if client == "" || client == "unknown" {
		return nil, nil
	}
	command, err := protocol.ParseCommand(payload)
	if err != nil {
		return nil, nil
	}
	s.mu.Lock()
	if auth, ok := command.(protocol.Auth); ok {
		owner := &control.Owner{Name: "Bluetooth"}
		owner.Revoke = func(reason string) {
			s.operations.Lock()
			defer s.operations.Unlock()
			s.mu.Lock()
			if s.owner != owner {
				s.mu.Unlock()
				return
			}
			s.authed, s.owner = "", nil
			s.mu.Unlock()
			s.hooks.OnActions([]Action{{Notify: mustReply(protocol.Bye{Reason: reason}), Client: client}, {Drop: client}})
		}
		accepted, revoke := s.hooks.Control.Claim(auth.Code, owner)
		if !accepted {
			if s.authed == client {
				s.hooks.Control.Release(s.owner)
				s.authed, s.owner = "", nil
			}
			s.mu.Unlock()
			return []Action{{Notify: mustReply(protocol.Bye{Reason: protocol.ReasonBadAuth}), Client: client, Drop: client}}, nil
		}
		old := s.authed
		s.authed, s.owner = client, owner
		s.mu.Unlock()
		var actions []Action
		if old != "" && old != client {
			actions = append(actions, Action{Drop: old, Reason: protocol.ReasonDisplaced})
		}
		return append(actions, Action{Notify: mustReply(protocol.Status{}), Client: client}), revoke
	}
	if s.authed != client {
		s.mu.Unlock()
		return []Action{{Drop: client, Reason: protocol.ReasonBadAuth}}, nil
	}
	owner := s.owner
	s.mu.Unlock()
	if move, ok := command.(protocol.Move); ok && !protocol.MoveInRange(move.Dx, move.Dy) {
		return nil, nil
	}
	if scroll, ok := command.(protocol.Scroll); ok && !protocol.MoveInRange(scroll.Dx, scroll.Dy) {
		return nil, nil
	}
	s.hooks.Control.Dispatch(owner, func() { s.hooks.OnCommand(command) })
	return nil, nil
}

func (s *Session) Kick() []Action {
	s.operations.Lock()
	defer s.operations.Unlock()
	return s.kick()
}

func (s *Session) Close() {
	s.operations.Lock()
	defer s.operations.Unlock()
	s.closed = true
	s.kick()
}

func (s *Session) kick() []Action {
	s.mu.Lock()
	authed, owner := s.authed, s.owner
	s.authed, s.owner = "", nil
	s.mu.Unlock()
	s.hooks.Control.Release(owner)
	if authed == "" {
		return nil
	}
	actions := []Action{{Notify: mustReply(protocol.Bye{Reason: protocol.ReasonKicked}), Client: authed}, {Drop: authed}}
	s.hooks.OnActions(actions)
	return actions
}

func (s *Session) Drop(client string) {
	s.operations.Lock()
	defer s.operations.Unlock()
	s.mu.Lock()
	if s.authed != client {
		s.mu.Unlock()
		return
	}
	owner := s.owner
	s.authed, s.owner = "", nil
	s.mu.Unlock()
	s.hooks.Control.Release(owner)
}

func (s *Session) Authed() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.authed
}

func mustReply(reply protocol.Reply) []byte {
	data, _ := protocol.EncodeReply(reply)
	return data
}
