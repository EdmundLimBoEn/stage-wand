package server

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func startHTTPTestServer(t *testing.T, cfg Config) string {
	t.Helper()
	cfg.Ports = []int{0}
	srv := New(NewSession("0000"), cfg, nil)
	port, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
}

func httpConn(t *testing.T, addr string) net.Conn {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	c.SetDeadline(time.Now().Add(2 * time.Second))
	return c
}

func request(t *testing.T, c net.Conn) {
	t.Helper()
	if _, err := io.WriteString(c, "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		t.Fatal(err)
	}
}

func expectClosed(t *testing.T, c net.Conn) {
	t.Helper()
	var b [1]byte
	_, err := c.Read(b[:])
	if err == nil {
		t.Fatal("expected connection closure")
	}
	if e, ok := err.(net.Error); ok && e.Timeout() {
		t.Fatal("connection remained open until client timeout")
	}
}

func TestHTTPIdleTimeout(t *testing.T) {
	addr := startHTTPTestServer(t, Config{IdleTimeout: 50 * time.Millisecond})
	c := httpConn(t, addr)
	request(t, c)
	expectClosed(t, c)
}

func TestHTTPReadTimeoutBoundsBodyDraining(t *testing.T) {
	addr := startHTTPTestServer(t, Config{ReadTimeout: 50 * time.Millisecond})
	c := httpConn(t, addr)
	if _, err := io.WriteString(c, "POST / HTTP/1.1\r\nHost: localhost\r\nContent-Length: 100\r\n\r\nx"); err != nil {
		t.Fatal(err)
	}
	// The handler rejects the upgrade; net/http must not drain this unfinished body forever.
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	expectClosed(t, c)
}

func TestHTTPAdmissionCap(t *testing.T) {
	addr := startHTTPTestServer(t, DefaultConfig())
	conns := make([]net.Conn, 128)
	for i := range conns {
		conns[i] = httpConn(t, addr)
		request(t, conns[i]) // Prove each connection was admitted and remains keep-alive.
	}
	extra := httpConn(t, addr)
	if _, err := io.WriteString(extra, "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	extra.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	var b [1]byte
	if _, err := extra.Read(b[:]); err == nil {
		t.Fatal("129th connection was admitted")
	} else if e, ok := err.(net.Error); !ok || !e.Timeout() {
		t.Fatalf("expected blocked admission: %v", err)
	}
	conns[0].Close()
	extra.SetReadDeadline(time.Now().Add(time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(extra), nil)
	if err != nil {
		t.Fatalf("slot not released: %v", err)
	}
	resp.Body.Close()
}

func TestHTTPHeaderLimit(t *testing.T) {
	addr := startHTTPTestServer(t, DefaultConfig())
	c := httpConn(t, addr)
	if _, err := fmt.Fprintf(c, "GET / HTTP/1.1\r\nHost: localhost\r\nX-Large: %s\r\n\r\n", strings.Repeat("x", 32*1024)); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestHeaderFieldsTooLarge {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestWebSocketOutlivesHTTPReadTimeout(t *testing.T) {
	addr := startHTTPTestServer(t, Config{ReadTimeout: 50 * time.Millisecond})
	c := dial(t, "ws://"+addr+"/")
	if err := c.WriteJSON(map[string]string{"t": "auth", "code": "0000"}); err != nil {
		t.Fatal(err)
	}
	readJSON(t, c, time.Second)
	time.Sleep(100 * time.Millisecond)
	if err := c.WriteJSON(map[string]string{"t": "auth", "code": "0000"}); err != nil {
		t.Fatal(err)
	}
	if frame := readJSON(t, c, time.Second); frame["t"] != "status" {
		t.Fatalf("got %v", frame)
	}
}
