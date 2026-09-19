package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFixturesRoundTrip(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "..", "Shared", "protocol-fixtures.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		Commands        []json.RawMessage `json:"commands"`
		Replies         []json.RawMessage `json:"replies"`
		InvalidCommands []string          `json:"invalidCommands"`
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, frame := range fixtures.Commands {
		command, err := ParseCommand(frame)
		if err != nil {
			t.Fatalf("parse command %s: %v", frame, err)
		}
		encoded, err := EncodeCommand(command)
		if err != nil {
			t.Fatal(err)
		}
		again, err := ParseCommand(encoded)
		if err != nil {
			t.Fatal(err)
		}
		first, err := EncodeCommand(command)
		if err != nil {
			t.Fatal(err)
		}
		second, err := EncodeCommand(again)
		if err != nil {
			t.Fatal(err)
		}
		if string(first) != string(second) {
			t.Fatalf("encode mismatch %s vs %s", first, second)
		}
	}
	for _, frame := range fixtures.Replies {
		reply, err := ParseReply(frame)
		if err != nil {
			t.Fatalf("parse reply %s: %v", frame, err)
		}
		encoded, err := EncodeReply(reply)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseReply(encoded); err != nil {
			t.Fatal(err)
		}
	}
	for _, frame := range fixtures.InvalidCommands {
		if _, err := ParseCommand([]byte(frame)); err == nil {
			t.Fatalf("accepted invalid command %s", frame)
		}
	}
}
