//go:build linux

package input

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/EdmundLimBoEn/stage-wand/host/internal/protocol"
)

type eventDevice struct {
	events []inputEvent
	writes int
	failAt int
	closed bool
}

var errEventWrite = errors.New("event write failed")

func (d *eventDevice) Write(data []byte) (int, error) {
	d.writes++
	if d.writes == d.failAt {
		return 0, errEventWrite
	}
	var event inputEvent
	if err := binary.Read(bytes.NewReader(data), nativeEndian, &event); err != nil {
		return 0, err
	}
	d.events = append(d.events, event)
	return len(data), nil
}

func (d *eventDevice) Close() error { d.closed = true; return nil }
func (d *eventDevice) Fd() uintptr  { return ^uintptr(0) }

func TestUInputReleasesKeysAfterEveryWriteFailure(t *testing.T) {
	for _, test := range []struct {
		command protocol.Command
		writes  int
	}{
		{protocol.Click{Button: protocol.ButtonLeft}, 4},
		{protocol.Click{Button: protocol.ButtonRight}, 4},
		{protocol.KeyPress{Key: protocol.KeyRight}, 4},
		{protocol.ChordPress{Chord: protocol.SpaceRight}, 8},
	} {
		for failure := 1; failure <= test.writes; failure++ {
			t.Run(fmt.Sprintf("%T-%d", test.command, failure), func(t *testing.T) {
				device := &eventDevice{failAt: failure}
				injector := &UInput{file: device, probe: Probe{Writable: true}}
				err := injector.Apply(test.command)
				if !errors.Is(err, errEventWrite) {
					t.Fatalf("lost failure: %v", err)
				}
				if ready, _ := injector.Ready(); ready {
					t.Fatal("failed injector reports ready")
				}
				if err := injector.Apply(protocol.Move{}); err != nil {
					t.Fatal(err)
				}
				if ready, _ := injector.Ready(); ready {
					t.Fatal("empty movement cleared the injection failure")
				}
				held := map[uint16]bool{}
				for _, event := range device.events {
					if event.Type == evKey {
						held[event.Code] = event.Value != 0
					}
				}
				for key, down := range held {
					if down {
						t.Fatalf("key %d left pressed: %v", key, device.events)
					}
				}
				if err := injector.Apply(protocol.Move{Dx: 1}); err != nil {
					t.Fatal(err)
				}
				if ready, _ := injector.Ready(); !ready {
					t.Fatal("successful injection did not restore readiness")
				}
			})
		}
	}
}

func TestUInputRetriesFailedLegacyWheelEvent(t *testing.T) {
	device := &eventDevice{failAt: 2}
	injector := &UInput{file: device}
	if err := injector.Apply(protocol.Scroll{Dy: 120}); !errors.Is(err, errEventWrite) {
		t.Fatalf("lost failure: %v", err)
	}
	if err := injector.Apply(protocol.Scroll{Dy: 1}); err != nil {
		t.Fatal(err)
	}
	var detents int32
	for _, event := range device.events {
		if event.Type == evRel && event.Code == relWheel {
			detents += event.Value
		}
	}
	if detents != 1 || injector.wheelY != 1 {
		t.Fatalf("dropped legacy scroll: detents=%d remainder=%d", detents, injector.wheelY)
	}
}

func TestUInputFlushesPartialRelativeFrames(t *testing.T) {
	for _, test := range []struct {
		command protocol.Command
		writes  int
	}{
		{protocol.Move{Dx: 10, Dy: 20}, 3},
		{protocol.Scroll{Dx: 120, Dy: 240}, 5},
	} {
		for failure := 1; failure <= test.writes; failure++ {
			t.Run(fmt.Sprintf("%T-%d", test.command, failure), func(t *testing.T) {
				device := &eventDevice{failAt: failure}
				injector := &UInput{file: device, probe: Probe{Writable: true}}
				if err := injector.Apply(test.command); !errors.Is(err, errEventWrite) {
					t.Fatalf("lost failure: %v", err)
				}
				if len(device.events) == 0 || device.events[len(device.events)-1].Type != evSyn {
					t.Fatalf("partial movement not synchronized: %v", device.events)
				}
				if ready, _ := injector.Ready(); ready {
					t.Fatal("synchronizing a partial frame hid the input failure")
				}
			})
		}
	}
}

func TestUInputAccumulatesLegacyWheelDetents(t *testing.T) {
	device := &eventDevice{}
	injector := &UInput{file: device}
	for _, delta := range []protocol.Scroll{
		{Dx: 40, Dy: -30}, {Dx: 40, Dy: -30}, {Dx: 40, Dy: -30}, {Dy: -30},
		{Dx: -80, Dy: 80}, {Dx: 40, Dy: -40}, {Dx: -80, Dy: 80},
	} {
		if err := injector.Apply(delta); err != nil {
			t.Fatal(err)
		}
	}
	var vertical, horizontal []int32
	for _, event := range device.events {
		switch event.Code {
		case relWheel:
			vertical = append(vertical, event.Value)
		case relHWheel:
			horizontal = append(horizontal, event.Value)
		}
	}
	if fmt.Sprint(vertical) != "[-1 1]" || fmt.Sprint(horizontal) != "[1 -1]" {
		t.Fatalf("legacy wheel steps vertical=%v horizontal=%v", vertical, horizontal)
	}
	if injector.wheelX != 0 || injector.wheelY != 0 {
		t.Fatalf("unexpected remainder %d,%d", injector.wheelX, injector.wheelY)
	}
}

func TestUInputCloseIsSafeDuringCommands(t *testing.T) {
	device := &eventDevice{}
	injector := &UInput{file: device, probe: Probe{Writable: true}}
	var workers sync.WaitGroup
	for i := 0; i < 10; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < 20; j++ {
				if err := injector.Apply(protocol.KeyPress{Key: protocol.KeyEsc}); err != nil && !errors.Is(err, os.ErrClosed) {
					t.Error(err)
				}
				injector.Ready()
			}
		}()
	}
	if err := injector.Close(); err != nil {
		t.Fatal(err)
	}
	workers.Wait()
	if err := injector.Close(); err != nil {
		t.Fatal(err)
	}
	if ready, _ := injector.Ready(); ready || !device.closed {
		t.Fatal("closed device reports ready or was not closed")
	}
}
