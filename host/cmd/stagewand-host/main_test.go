package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/EdmundLimBoEn/stage-wand/host/internal/server"
	"io"
	"testing"

	"github.com/EdmundLimBoEn/stage-wand/host/internal/input"
	"github.com/EdmundLimBoEn/stage-wand/host/internal/protocol"
)

func TestPairingCodeValidation(t *testing.T) {
	for _, code := range []string{"0000", "0012", "9999"} {
		if !validCode(code) {
			t.Fatalf("rejected %q", code)
		}
	}
	for _, code := range []string{"", "123", "12345", " 123", "１２３４", "abcd", "12\n3"} {
		if validCode(code) {
			t.Fatalf("accepted %q", code)
		}
	}
}

func TestPairingCodeEntropyFailureIsNotAValidCode(t *testing.T) {
	code, err := generateCode(bytes.NewReader(nil), "")
	if !errors.Is(err, io.EOF) || code != "" {
		t.Fatalf("code=%q err=%v", code, err)
	}
}

func TestPairingCodeRotationNeverReusesCurrentCode(t *testing.T) {
	for _, previous := range []string{"0000", "0012", "9999"} {
		for _, entropy := range [][]byte{{0, 0}, {0, 12}, {0x27, 0x0e}} {
			code, err := generateCode(bytes.NewReader(entropy), previous)
			if err != nil || !validCode(code) || code == previous {
				t.Fatalf("previous=%q code=%q err=%v", previous, code, err)
			}
		}
	}
}

func TestUnavailableInputCannotReportReady(t *testing.T) {
	injector, _ := openInjector(true)
	if ready, note := injector.Ready(); ready || note == "" {
		t.Fatalf("dry run ready=%v note=%q", ready, note)
	}
	if ready, note := (unavailableInput{reason: "permission denied"}).Ready(); ready || note != "permission denied" {
		t.Fatalf("missing input ready=%v note=%q", ready, note)
	}
}

type failingInjector struct {
	input.Logging
	failure  error
	commands int
}

func (f *failingInjector) Apply(command protocol.Command) error {
	f.commands++
	if _, ok := command.(protocol.KeyPress); ok {
		return f.failure
	}
	return nil
}

func TestSelfTestReportsKeyboardFailure(t *testing.T) {
	failure := errors.New("SendInput rejected keyboard event")
	injector := &failingInjector{failure: failure}
	if err := applySelfTest(injector); !errors.Is(err, failure) {
		t.Fatalf("got %v", err)
	}
	if injector.commands != 4 {
		t.Fatalf("continued after failure: %d commands", injector.commands)
	}
}

func TestSelfTestPostsEveryCommand(t *testing.T) {
	count := 0
	err := applySelfTest(input.Logging{Log: func(protocol.Command) { count++ }})
	if err != nil || count != 5 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestDesktopStatusPreservesCodeAndReadiness(t *testing.T) {
	session := server.NewSession("0012")
	status := desktopStatus(session, "192.0.2.1", 8788, false, "permission denied", "advertised", "unavailable")
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	var decoded DesktopStatus
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Code != "0012" || decoded.InputReady || decoded.Address != "192.0.2.1:8788" || decoded.Peer != "" || decoded.Type != "status" {
		t.Fatalf("bad desktop status: %+v", decoded)
	}
	session.SetCode("0034")
	if desktopStatus(session, "", 8787, true, "", "", "").Code != "0034" {
		t.Fatal("stale pairing code")
	}
}
