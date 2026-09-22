package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/EdmundLimBoEn/stage-wand/host/internal/protocol"
	"github.com/gorilla/websocket"
)

func TestOriginPolicyAllowsNativeAndSameHostOnly(t *testing.T) {
	raw, _ := startServer(t, nil)
	endpoint, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{"", "http://" + endpoint.Host} {
		headers := http.Header{}
		if origin != "" {
			headers.Set("Origin", origin)
		}
		conn, _, err := websocket.DefaultDialer.Dial(raw, headers)
		if err != nil {
			t.Fatalf("origin %q: %v", origin, err)
		}
		conn.Close()
	}
	conn, response, err := websocket.DefaultDialer.Dial(raw, http.Header{"Origin": {"https://unrelated.example"}})
	if conn != nil {
		conn.Close()
	}
	if err == nil || response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin request accepted: response=%v err=%v", response, err)
	}
}

func TestRepeatedWrongCodesCannotDisplaceCurrentController(t *testing.T) {
	raw, srv := startServer(t, nil)
	current := dial(t, raw)
	current.WriteJSON(map[string]string{"t": "auth", "code": "0000"})
	if readJSON(t, current, time.Second)["t"] != "status" {
		t.Fatal("auth failed")
	}
	owner := srv.session.Peer()
	for i := 0; i < 5; i++ {
		bad := dial(t, raw)
		bad.WriteJSON(map[string]string{"t": "auth", "code": "9999"})
		if readJSON(t, bad, time.Second)["reason"] != "badauth" {
			t.Fatal("wrong code accepted")
		}
	}
	blocked := dial(t, raw)
	blocked.WriteJSON(map[string]string{"t": "auth", "code": "0000"})
	if readJSON(t, blocked, time.Second)["reason"] != "badauth" {
		t.Fatal("lockout bypassed")
	}
	if srv.session.Peer() != owner {
		t.Fatal("lockout displaced current controller")
	}
	srv.session.SetCode("0012")
	next := dial(t, raw)
	next.WriteJSON(map[string]string{"t": "auth", "code": "0012"})
	if readJSON(t, next, time.Second)["t"] != "status" {
		t.Fatal("new code could not pair")
	}
}

func TestCloseRejectsNewConnectionsAndReleasesController(t *testing.T) {
	raw, srv := startServer(t, nil)
	conn := dial(t, raw)
	conn.WriteJSON(map[string]string{"t": "auth", "code": "0000"})
	readJSON(t, conn, time.Second)
	if err := srv.Close(); err != nil {
		t.Fatal(err)
	}
	if srv.session.Peer() != "" {
		t.Fatal("shutdown retained ownership")
	}
	if frame := readJSON(t, conn, time.Second); frame["reason"] != "kicked" {
		t.Fatalf("shutdown: %v", frame)
	}
	other, _, err := websocket.DefaultDialer.Dial(raw, nil)
	if other != nil {
		other.Close()
	}
	if err == nil {
		t.Fatal("server accepted after close")
	}
	if err := srv.Close(); err != nil {
		t.Fatalf("repeated close: %v", err)
	}
}

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
	if err := conn.WriteJSON(map[string]string{"t": "auth", "code": "9999"}); err != nil {
		t.Fatal(err)
	}
	frame := readJSON(t, conn, 2*time.Second)
	if frame["t"] != "bye" || frame["reason"] != "badauth" {
		t.Fatalf("got %#v", frame)
	}
}

func TestRepeatedAuthHasTheSameReplyAndValidation(t *testing.T) {
	raw, srv := startServer(t, nil)
	conn := dial(t, raw)
	for i := 0; i < 2; i++ {
		if err := conn.WriteJSON(map[string]string{"t": "auth", "code": "0000"}); err != nil {
			t.Fatal(err)
		}
		if frame := readJSON(t, conn, time.Second); frame["t"] != "status" {
			t.Fatalf("auth %d: %v", i, frame)
		}
	}
	if err := conn.WriteJSON(map[string]string{"t": "auth", "code": "9999"}); err != nil {
		t.Fatal(err)
	}
	if frame := readJSON(t, conn, time.Second); frame["reason"] != "badauth" {
		t.Fatalf("bad reauth: %v", frame)
	}
	deadline := time.Now().Add(time.Second)
	for srv.session.Peer() != "" {
		if time.Now().After(deadline) {
			t.Fatal("bad reauth retained controller")
		}
		time.Sleep(time.Millisecond)
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
