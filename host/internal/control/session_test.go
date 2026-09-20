package control

import (
	"sync"
	"testing"
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
