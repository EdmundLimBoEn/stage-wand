package protocol

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
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
		ValidCommandTexts []string          `json:"validCommandTexts"`
		Commands          []json.RawMessage `json:"commands"`
		Replies           []json.RawMessage `json:"replies"`
		InvalidCommands   []string          `json:"invalidCommands"`
		InvalidReplies    []string          `json:"invalidReplies"`
		Bluetooth         struct {
			MaxFrameBytes       int `json:"maxFrameBytes"`
			MinimumCommandBytes int `json:"minimumCommandBytes"`
		} `json:"bluetooth"`
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	if fixtures.Bluetooth.MaxFrameBytes != MaxFrameBytes || fixtures.Bluetooth.MinimumCommandBytes != MinimumCommandBytes {
		t.Fatal("BLE wire size constants drifted")
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
		assertJSONEqual(t, frame, encoded)
		if len(encoded) > MinimumCommandBytes {
			t.Fatalf("canonical command exceeds minimum BLE payload: %s", encoded)
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
		assertJSONEqual(t, frame, encoded)
		if _, err := ParseReply(encoded); err != nil {
			t.Fatal(err)
		}
	}
	for _, frame := range fixtures.ValidCommandTexts {
		command, err := ParseCommand([]byte(frame))
		if err != nil {
			t.Fatalf("rejected valid text %q: %v", frame, err)
		}
		encoded, err := EncodeCommand(command)
		if err != nil {
			t.Fatal(err)
		}
		assertJSONEqual(t, []byte(frame), encoded)
	}
	for _, frame := range fixtures.InvalidCommands {
		if _, err := ParseCommand([]byte(frame)); err == nil {
			t.Fatalf("accepted invalid command %s", frame)
		}
	}
	for _, frame := range fixtures.InvalidReplies {
		if _, err := ParseReply([]byte(frame)); err == nil {
			t.Fatalf("accepted invalid reply %s", frame)
		}
	}
}

func assertJSONEqual(t *testing.T, want, got []byte) {
	t.Helper()
	var expected, actual any
	if err := json.Unmarshal(want, &expected); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(got, &actual); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("frame changed: %s -> %s", want, got)
	}
}

func TestEncoderRejectsInvalidCommands(t *testing.T) {
	for _, command := range []Command{Auth{Code: "\xff"}, Click{Button: "middle"}, KeyPress{Key: "enter"}, ChordPress{Chord: "unknown"}, Move{Dx: math.Inf(1)}, Scroll{Dy: math.NaN()}} {
		if _, err := EncodeCommand(command); err == nil {
			t.Fatalf("encoded invalid command: %#v", command)
		}
	}
}

func TestInvalidUTF8Rejected(t *testing.T) {
	if _, err := EncodeReply(Bye{Reason: "\xff"}); err == nil {
		t.Fatal("encoded invalid UTF-8 reply")
	}
	if _, err := ParseCommand([]byte("{\"t\":\"auth\",\"code\":\"\xff\"}")); err == nil {
		t.Fatal("accepted invalid UTF-8 command")
	}
	if _, err := ParseReply([]byte("{\"t\":\"bye\",\"reason\":\"\xff\"}")); err == nil {
		t.Fatal("accepted invalid UTF-8 reply")
	}
}

func FuzzParseCommand(f *testing.F) {
	for _, frame := range []string{`{"t":"auth","code":"4821"}`, `{"t":"move","dx":1,"dy":0}`, `null`, `{"t":[]}`} {
		f.Add([]byte(frame))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		command, err := ParseCommand(data)
		if err != nil {
			return
		}
		encoded, err := EncodeCommand(command)
		if err != nil {
			t.Fatalf("accepted command cannot encode: %v", err)
		}
		again, err := ParseCommand(encoded)
		if err != nil || !reflect.DeepEqual(command, again) {
			t.Fatalf("accepted command failed round-trip: %s (%v)", data, err)
		}
	})
}
