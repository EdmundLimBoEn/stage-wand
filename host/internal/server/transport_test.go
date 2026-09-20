package server

import (
	"sync"
	"testing"
	"time"

	"github.com/EdmundLimBoEn/stage-wand/host/internal/ble"
	"github.com/EdmundLimBoEn/stage-wand/host/internal/protocol"
)

func TestBluetoothAndLANShareOneController(t *testing.T) {
	commands := make(chan protocol.Command, 10)
	apply := func(command protocol.Command) { commands <- command }
	raw, srv := startServer(t, apply)
	var mu sync.Mutex
	var displaced []ble.Action
	radio := ble.NewSession(ble.Hooks{Control: srv.session, OnCommand: apply, OnActions: func(actions []ble.Action) {
		mu.Lock()
		if len(actions) == 2 && actions[1].Drop == "phone" {
			displaced = append(displaced, actions...)
		}
		mu.Unlock()
	}})
	lan := dial(t, raw)
	if err := lan.WriteJSON(map[string]string{"t": "auth", "code": "0000"}); err != nil {
		t.Fatal(err)
	}
	if readJSON(t, lan, time.Second)["t"] != "status" {
		t.Fatal("LAN auth failed")
	}
	radio.Handle("phone", []byte(`{"t":"auth","code":"0000"}`))
	if frame := readJSON(t, lan, time.Second); frame["reason"] != "displaced" {
		t.Fatalf("LAN not displaced: %v", frame)
	}
	radio.Handle("phone", []byte(`{"t":"key","k":"right"}`))
	select {
	case <-commands:
	case <-time.After(time.Second):
		t.Fatal("BLE did not inject")
	}
	second := dial(t, raw)
	if err := second.WriteJSON(map[string]string{"t": "auth", "code": "0000"}); err != nil {
		t.Fatal(err)
	}
	if readJSON(t, second, time.Second)["t"] != "status" {
		t.Fatal("second LAN auth failed")
	}
	if radio.Authed() != "" {
		t.Fatal("BLE still authorized")
	}
	mu.Lock()
	notified := len(displaced) == 2 && displaced[1].Drop == "phone"
	mu.Unlock()
	if !notified {
		t.Fatal("BLE did not receive displacement")
	}
	radio.Handle("phone", []byte(`{"t":"key","k":"left"}`))
	if err := second.WriteJSON(map[string]string{"t": "key", "k": "right"}); err != nil {
		t.Fatal(err)
	}
	select {
	case command := <-commands:
		if command.(protocol.KeyPress).Key != protocol.KeyRight {
			t.Fatal("displaced BLE injected")
		}
	case <-time.After(time.Second):
		t.Fatal("LAN did not inject")
	}
	if len(commands) != 0 {
		t.Fatal("extra injection")
	}
	srv.session.SetCode("1234")
	if frame := readJSON(t, second, time.Second); frame["reason"] != "kicked" {
		t.Fatalf("kick: %v", frame)
	}
	if srv.session.Peer() != "" {
		t.Fatal("kick left an owner")
	}
}
