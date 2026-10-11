package control

import (
	"sync"
	"testing"
	"time"
)

func TestStaleOwnerCannotInjectOrClearNewPeer(t *testing.T) {
	session := NewSession("0000")
	first := &Owner{Name: "Wi-Fi"}
	second := &Owner{Name: "Bluetooth"}
	session.Claim("0000", first)
	session.Claim("0000", second)
	session.Release(first)
	if session.Dispatch(first, func() { t.Fatal("stale injection") }) {
		t.Fatal("stale owner accepted")
	}
	if session.Peer() != "Bluetooth" {
		t.Fatal("stale disconnect cleared new peer")
	}
	session.SetCode("1234")
	if session.Dispatch(second, func() { t.Fatal("kicked injection") }) {
		t.Fatal("kicked owner accepted")
	}
	if ok, _ := session.Claim("0000", first); ok {
		t.Fatal("old code accepted")
	}
}

func TestAuthFailuresShareBudgetAcrossTransports(t *testing.T) {
	session := NewSession("0012")
	now := time.Unix(100, 0)
	session.now = func() time.Time { return now }
	current := &Owner{Name: "connected"}
	session.Claim("0012", current)
	for i := 0; i < authFailureLimit; i++ {
		if ok, _ := session.Claim("1234", &Owner{Name: "new connection"}); ok {
			t.Fatal("wrong code accepted")
		}
	}
	if ok, _ := session.Claim("0012", &Owner{Name: "Bluetooth"}); ok {
		t.Fatal("new transport bypassed lockout")
	}
	if !session.Dispatch(current, func() {}) {
		t.Fatal("failed pairing interrupted current controller")
	}
	now = now.Add(authFailureWindow)
	if ok, _ := session.Claim("0012", &Owner{Name: "Bluetooth"}); !ok {
		t.Fatal("lockout did not expire")
	}
}

func TestCodeRotationClearsAuthLockout(t *testing.T) {
	session := NewSession("0012")
	owner := &Owner{Name: "phone"}
	for i := 0; i < authFailureLimit; i++ {
		session.Claim("1234", owner)
	}
	session.SetCode("0099")
	if ok, _ := session.Claim("0099", owner); !ok {
		t.Fatal("code rotation retained lockout")
	}
	if ok, _ := session.Claim("0099", nil); ok {
		t.Fatal("nil owner accepted")
	}
}

func TestConcurrentClaimsLeaveExactlyOneOwner(t *testing.T) {
	session := NewSession("0000")
	owners := make([]*Owner, 100)
	var wg sync.WaitGroup
	for i := range owners {
		owners[i] = &Owner{Name: "phone"}
		wg.Add(1)
		go func(owner *Owner) { defer wg.Done(); session.Claim("0000", owner) }(owners[i])
	}
	wg.Wait()
	applied := 0
	for _, owner := range owners {
		session.Dispatch(owner, func() { applied++ })
	}
	if applied != 1 {
		t.Fatalf("%d controllers", applied)
	}
}
