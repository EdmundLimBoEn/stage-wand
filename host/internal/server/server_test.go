package server

import (
	"encoding/json"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/EdmundLimBoEn/stage-wand/host/internal/protocol"
	"github.com/gorilla/websocket"
)

func startServer(t *testing.T, onCommand func(protocol.Command)) (string, *Server) {
	t.Helper()
	session := NewSession("0000")
	cfg := DefaultConfig()
	cfg.Ports = []int{0}
	srv := New(session, cfg, onCommand)
	t.Cleanup(func() { srv.Close() })
	port, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	return "ws://127.0.0.1:" + strconv.Itoa(port) + "/", srv
}

func dial(t *testing.T, raw string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func readJSON(t *testing.T, conn *websocket.Conn, timeout time.Duration) map[string]any {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(timeout))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var frame map[string]any
	if err := json.Unmarshal(data, &frame); err != nil {
		t.Fatal(err)
	}
	return frame
}

func TestNoAuthCloses(t *testing.T) {
	raw, _ := startServer(t, nil)
	conn := dial(t, raw)
	started := time.Now()
	conn.SetReadDeadline(time.Now().Add(4 * time.Second))
	_, _, err := conn.ReadMessage()
	if err == nil {
		t.Fatal("expected close")
	}
	elapsed := time.Since(started)
	if elapsed < 1500*time.Millisecond || elapsed > 3500*time.Millisecond {
		t.Fatalf("auth deadline %s", elapsed)
	}
}

func TestBadAuth(t *testing.T) {
	raw, _ := startServer(t, nil)
	conn := dial(t, raw)
	if err := conn.WriteJSON(map[string]string{"t": "auth", "code": "wrong"}); err != nil {
		t.Fatal(err)
	}
	frame := readJSON(t, conn, 2*time.Second)
	if frame["t"] != "bye" || frame["reason"] != "badauth" {
		t.Fatalf("got %#v", frame)
	}
}

func TestNonAuthFirstFrame(t *testing.T) {
	raw, _ := startServer(t, nil)
	conn := dial(t, raw)
	if err := conn.WriteJSON(map[string]string{"t": "key", "k": "right"}); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("expected close")
	}
}

func TestGoodAuthDisplaceAndMoves(t *testing.T) {
	var mu sync.Mutex
	var got []protocol.Command
	raw, _ := startServer(t, func(command protocol.Command) {
		mu.Lock()
		got = append(got, command)
		mu.Unlock()
	})
	first := dial(t, raw)
	if err := first.WriteJSON(map[string]string{"t": "auth", "code": "0000"}); err != nil {
		t.Fatal(err)
	}
	if readJSON(t, first, 2*time.Second)["t"] != "status" {
		t.Fatal("first auth")
	}
	second := dial(t, raw)
	if err := second.WriteJSON(map[string]string{"t": "auth", "code": "0000"}); err != nil {
		t.Fatal(err)
	}
	if readJSON(t, second, 2*time.Second)["t"] != "status" {
		t.Fatal("second auth")
	}
	bye := readJSON(t, first, 2*time.Second)
	if bye["t"] != "bye" || bye["reason"] != "displaced" {
		t.Fatalf("got %#v", bye)
	}
	if err := second.WriteMessage(websocket.TextMessage, []byte(`{"t":"move","dx":1e999,"dy":0}`)); err != nil {
		t.Fatal(err)
	}
	if err := second.WriteJSON(map[string]any{"t": "move", "dx": 401, "dy": 0}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 200; i++ {
		dy := 0
		if i != 0 {
			dy = -i
		}
		if err := second.WriteJSON(map[string]any{"t": "move", "dx": i, "dy": dy}); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n >= 200 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("got %d moves", n)
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 200 {
		t.Fatalf("got %d", len(got))
	}
	for i, command := range got {
		move, ok := command.(protocol.Move)
		if !ok {
			t.Fatalf("command %d %T", i, command)
		}
		wantY := 0.0
		if i != 0 {
			wantY = float64(-i)
		}
		if move.Dx != float64(i) || move.Dy != wantY {
			t.Fatalf("move %d = %#v", i, move)
		}
	}
}
