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
	Notify    []byte
	Broadcast bool
	Drop      string
	Reason    string
}

type Session struct {
	hooks      Hooks
	mu         sync.Mutex
	operations sync.Mutex
	authed     string
	owner      *control.Owner
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
			s.hooks.OnActions([]Action{{Notify: mustReply(protocol.Bye{Reason: reason}), Broadcast: true}, {Drop: client}})
		}
		accepted, revoke := s.hooks.Control.Claim(auth.Code, owner)
		if !accepted {
			idle := s.authed == ""
			if s.authed == client {
				s.hooks.Control.Release(s.owner)
				s.authed, s.owner = "", nil
			}
			s.mu.Unlock()
			if idle {
				return []Action{{Notify: mustReply(protocol.Bye{Reason: protocol.ReasonBadAuth}), Broadcast: true}}, nil
			}
			return []Action{{Drop: client}}, nil
		}
		old := s.authed
		s.authed, s.owner = client, owner
		s.mu.Unlock()
		var actions []Action
		if old != "" && old != client {
			actions = append(actions, Action{Drop: old, Reason: protocol.ReasonDisplaced})
		}
		return append(actions, Action{Notify: mustReply(protocol.Status{})}), revoke
	}
	if s.authed != client {
		s.mu.Unlock()
		return nil, nil
	}
	owner := s.owner
	s.mu.Unlock()
	if move, ok := command.(protocol.Move); ok && !protocol.MoveInRange(move.Dx, move.Dy) {
		return nil, nil
	}
	s.hooks.Control.Dispatch(owner, func() { s.hooks.OnCommand(command) })
	return nil, nil
}

func (s *Session) Kick() []Action {
	s.operations.Lock()
	defer s.operations.Unlock()
	s.mu.Lock()
	authed, owner := s.authed, s.owner
	s.authed, s.owner = "", nil
	s.mu.Unlock()
	s.hooks.Control.Release(owner)
	if authed == "" {
		return nil
	}
	actions := []Action{{Notify: mustReply(protocol.Bye{Reason: protocol.ReasonKicked}), Broadcast: true}, {Drop: authed}}
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
