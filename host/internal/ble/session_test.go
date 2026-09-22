package ble

import (
	"sync"
	"testing"
	"time"

	"github.com/EdmundLimBoEn/stage-wand/host/internal/control"
	"github.com/EdmundLimBoEn/stage-wand/host/internal/protocol"
)

func TestUUIDsMatchApple(t *testing.T) {
	if ServiceUUID != "5A3E0001-8B6C-4B1E-9F8D-2C7A1D4E6F01" {
		t.Fatalf("service %s", ServiceUUID)
	}
	if CommandUUID != "5A3E0002-8B6C-4B1E-9F8D-2C7A1D4E6F01" {
		t.Fatalf("command %s", CommandUUID)
	}
	if ReplyUUID != "5A3E0003-8B6C-4B1E-9F8D-2C7A1D4E6F01" {
		t.Fatalf("reply %s", ReplyUUID)
	}
}

func TestAuthStatusAndCommands(t *testing.T) {
	var mu sync.Mutex
	var got []protocol.Command
	session := NewSession(Hooks{
		Control: control.NewSession("0000"),
		OnCommand: func(command protocol.Command) {
			mu.Lock()
			got = append(got, command)
			mu.Unlock()
		},
	})
	if actions := session.Handle("phone", []byte(`{"t":"key","k":"right"}`)); len(actions) != 1 || actions[0].Drop != "phone" {
		t.Fatalf("pre-auth command: %#v", actions)
	}
	actions := session.Handle("phone", []byte(`{"t":"auth","code":"0000"}`))
	if len(actions) != 1 || string(actions[0].Notify) != `{"t":"status"}` {
		t.Fatalf("auth %#v", actions)
	}
	if session.Authed() != "phone" {
		t.Fatalf("authed %q", session.Authed())
	}
	session.Handle("phone", []byte(`{"t":"move","dx":1,"dy":-2}`))
	session.Handle("phone", []byte(`{"t":"move","dx":401,"dy":0}`))
	session.Handle("phone", []byte(`{"t":"scroll","dx":0,"dy":401}`))
	session.Handle("phone", []byte(`{"t":"key","k":"right"}`))
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("got %#v", got)
	}
	move, ok := got[0].(protocol.Move)
	if !ok || move.Dx != 1 || move.Dy != -2 {
		t.Fatalf("move %#v", got[0])
	}
	if _, ok := got[1].(protocol.KeyPress); !ok {
		t.Fatalf("key %#v", got[1])
	}
}

func TestBadAuthTargetsRejectedClient(t *testing.T) {
	session := NewSession(Hooks{Control: control.NewSession("0000")})
	actions := session.Handle("intruder", []byte(`{"t":"auth","code":"9999"}`))
	if len(actions) != 1 || actions[0].Client != "intruder" || string(actions[0].Notify) != `{"t":"bye","reason":"badauth"}` {
		t.Fatalf("%#v", actions)
	}
}

func TestBadAuthDropsSecondCentral(t *testing.T) {
	session := NewSession(Hooks{Control: control.NewSession("0000")})
	session.Handle("phone", []byte(`{"t":"auth","code":"0000"}`))
	actions := session.Handle("other", []byte(`{"t":"auth","code":"1111"}`))
	if len(actions) != 1 || actions[0].Drop != "other" || actions[0].Client != "other" || string(actions[0].Notify) != `{"t":"bye","reason":"badauth"}` {
		t.Fatalf("%#v", actions)
	}
	if session.Authed() != "phone" {
		t.Fatalf("authed %q", session.Authed())
	}
}

func TestDisplaceDropsPrevious(t *testing.T) {
	session := NewSession(Hooks{Control: control.NewSession("0000")})
	session.Handle("first", []byte(`{"t":"auth","code":"0000"}`))
	actions := session.Handle("second", []byte(`{"t":"auth","code":"0000"}`))
	if len(actions) != 2 {
		t.Fatalf("%#v", actions)
	}
	if actions[0].Drop != "first" {
		t.Fatalf("drop %#v", actions[0])
	}
	if string(actions[1].Notify) != `{"t":"status"}` {
		t.Fatalf("status %#v", actions[1])
	}
	if session.Authed() != "second" {
		t.Fatalf("authed %q", session.Authed())
	}
}

func TestKickAndDrop(t *testing.T) {
	controller := control.NewSession("0000")
	session := NewSession(Hooks{
		Control: controller,
	})
	session.Handle("phone", []byte(`{"t":"auth","code":"0000"}`))
	if controller.Peer() != "Bluetooth" {
		t.Fatalf("peer %q", controller.Peer())
	}
	actions := session.Kick()
	if len(actions) != 2 || actions[0].Client != "phone" || string(actions[0].Notify) != `{"t":"bye","reason":"kicked"}` {
		t.Fatalf("kick %#v", actions)
	}
	if session.Authed() != "" || controller.Peer() != "" {
		t.Fatalf("after kick authed=%q peer=%q", session.Authed(), controller.Peer())
	}
	session.Handle("phone", []byte(`{"t":"auth","code":"0000"}`))
	session.Drop("other")
	if session.Authed() != "phone" {
		t.Fatal("drop other")
	}
	session.Drop("phone")
	if session.Authed() != "" {
		t.Fatal("drop phone")
	}
}

func TestReauthenticationWaitsForDisplacementCleanup(t *testing.T) {
	controller := control.NewSession("0000")
	cleaning, resume := make(chan struct{}), make(chan struct{})
	session := NewSession(Hooks{Control: controller, OnActions: func(actions []Action) {
		if len(actions) == 2 && actions[1].Drop == "phone" {
			close(cleaning)
			<-resume
		}
	}})
	session.Handle("phone", []byte(`{"t":"auth","code":"0000"}`))
	_, revoke := controller.Claim("0000", &control.Owner{Name: "LAN"})
	revoked := make(chan struct{})
	go func() { revoke(); close(revoked) }()
	<-cleaning
	authenticated := make(chan struct{})
	go func() { session.Handle("phone", []byte(`{"t":"auth","code":"0000"}`)); close(authenticated) }()
	select {
	case <-authenticated:
		t.Fatal("reauthentication raced with stale disconnect")
	case <-time.After(30 * time.Millisecond):
	}
	close(resume)
	<-revoked
	<-authenticated
	if session.Authed() != "phone" || controller.Peer() != "Bluetooth" {
		t.Fatal("cleanup revoked new session")
	}
}

func TestClosedSessionCannotBeReauthenticated(t *testing.T) {
	controller := control.NewSession("0000")
	dispatched := 0
	session := NewSession(Hooks{Control: controller, OnCommand: func(protocol.Command) { dispatched++ }})
	session.Handle("phone", []byte(`{"t":"auth","code":"0000"}`))
	session.Close()
	session.Close()
	session.Handle("phone", []byte(`{"t":"auth","code":"0000"}`))
	session.Handle("phone", []byte(`{"t":"key","k":"right"}`))
	if session.Authed() != "" || controller.Peer() != "" || dispatched != 0 {
		t.Fatalf("closed session accepted control: authed=%q peer=%q commands=%d", session.Authed(), controller.Peer(), dispatched)
	}
}

func TestBadReauthenticationReleasesOwner(t *testing.T) {
	controller := control.NewSession("0000")
	session := NewSession(Hooks{Control: controller})
	session.Handle("phone", []byte(`{"t":"auth","code":"0000"}`))
	actions := session.Handle("phone", []byte(`{"t":"auth","code":"9999"}`))
	if len(actions) != 1 || actions[0].Client != "phone" || actions[0].Drop != "phone" || string(actions[0].Notify) != `{"t":"bye","reason":"badauth"}` {
		t.Fatalf("bad auth did not reject the owner: %#v", actions)
	}
	if session.Authed() != "" || controller.Peer() != "" {
		t.Fatal("bad reauthentication retained ownership")
	}
}
