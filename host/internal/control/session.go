package control

import (
	"crypto/subtle"
	"sync"
	"time"
)

const authFailureLimit = 5
const authFailureWindow = 30 * time.Second

type Owner struct {
	Name   string
	Revoke func(string)
}

type Session struct {
	mu              sync.Mutex
	code            string
	owner           *Owner
	port            int
	now             func() time.Time
	authFailures    int
	authWindowStart time.Time
}

func NewSession(code string) *Session { return &Session{code: code, now: time.Now} }

func (s *Session) Code() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.code
}

func (s *Session) SetCode(code string) {
	s.mu.Lock()
	s.code = code
	s.authFailures = 0
	s.authWindowStart = time.Time{}
	old := s.owner
	s.owner = nil
	s.mu.Unlock()
	if old != nil && old.Revoke != nil {
		old.Revoke("kicked")
	}
}

// Claim returns the old owner's cleanup for execution outside transport locks.
func (s *Session) Claim(code string, owner *Owner) (bool, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if s.authWindowStart.IsZero() || now.Sub(s.authWindowStart) >= authFailureWindow {
		s.authWindowStart = now
		s.authFailures = 0
	}
	// Keep one budget across LAN and Bluetooth; reconnecting must not reset it.
	if s.authFailures >= authFailureLimit || owner == nil {
		return false, func() {}
	}
	if subtle.ConstantTimeCompare([]byte(code), []byte(s.code)) != 1 {
		s.authFailures++
		return false, func() {}
	}
	s.authFailures = 0
	s.authWindowStart = time.Time{}
	old := s.owner
	s.owner = owner
	if old != nil && old != owner && old.Revoke != nil {
		return true, func() { old.Revoke("displaced") }
	}
	return true, func() {}
}

func (s *Session) Release(owner *Owner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.owner == owner {
		s.owner = nil
	}
}

// Dispatch keeps ownership checks and injection atomic with handover and kick.
func (s *Session) Dispatch(owner *Owner, apply func()) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if owner == nil || s.owner != owner {
		return false
	}
	apply()
	return true
}

func (s *Session) Peer() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.owner == nil {
		return ""
	}
	return s.owner.Name
}

func (s *Session) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.port
}

func (s *Session) SetPort(port int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.port = port
}
