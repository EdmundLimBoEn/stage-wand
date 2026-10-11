//go:build windows

package input

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"unsafe"

	"github.com/EdmundLimBoEn/stage-wand/host/internal/protocol"
)

func TestInputABILayout(t *testing.T) {
	var record inputRecord
	expectedSize, expectedOffset := uintptr(40), uintptr(8)
	if unsafe.Sizeof(uintptr(0)) == 4 {
		expectedSize, expectedOffset = 28, 4
	}
	if unsafe.Sizeof(record) != expectedSize || unsafe.Offsetof(record.Data) != expectedOffset {
		t.Fatalf("INPUT size=%d union offset=%d", unsafe.Sizeof(record), unsafe.Offsetof(record.Data))
	}
	record.Type = inputKeyboard
	*(*keyboardData)(unsafe.Pointer(&record.Data)) = keyboardData{Vk: vkRight, Flags: keyUp}
	key := (*keyboardData)(unsafe.Pointer(&record.Data))
	if key.Vk != vkRight || key.Flags != keyUp {
		t.Fatalf("keyboard union: %#v", key)
	}
}

func TestSendInputReleasesChordAfterEveryFailure(t *testing.T) {
	for failure := 1; failure <= 6; failure++ {
		t.Run(fmt.Sprint(failure), func(t *testing.T) {
			injected := errors.New("SendInput blocked")
			calls := 0
			held := map[uint16]bool{}
			injector := &SendInputInjector{sendEvent: func(record inputRecord) error {
				calls++
				if calls == failure {
					return injected
				}
				key := (*keyboardData)(unsafe.Pointer(&record.Data))
				held[key.Vk] = key.Flags&keyUp == 0
				if (key.Vk == vkLWin || key.Vk == vkRight) && key.Flags&keyExtended == 0 {
					t.Errorf("extended key missing flag: %#v", key)
				}
				return nil
			}}
			if err := injector.Apply(protocol.ChordPress{Chord: protocol.SpaceRight}); !errors.Is(err, injected) {
				t.Fatalf("lost failure: %v", err)
			}
			if ready, _ := injector.Ready(); ready {
				t.Fatal("failed SendInput reports ready")
			}
			if err := injector.Apply(protocol.Move{}); err != nil {
				t.Fatal(err)
			}
			if ready, _ := injector.Ready(); ready {
				t.Fatal("empty movement cleared the injection failure")
			}
			for key, down := range held {
				if down {
					t.Fatalf("key %#x remains pressed", key)
				}
			}
		})
	}
}

func TestSendInputCloseRetriesStuckMouseRelease(t *testing.T) {
	blocked := true
	var mouseDown bool
	injected := errors.New("SendInput blocked")
	injector := &SendInputInjector{sendEvent: func(record inputRecord) error {
		switch record.Data.Flags {
		case mouseRightDown:
			mouseDown = true
		case mouseRightUp:
			if blocked {
				return injected
			}
			mouseDown = false
		default:
			t.Errorf("unexpected mouse event: %#v", record)
		}
		return nil
	}}
	if err := injector.Apply(protocol.Click{Button: protocol.ButtonRight}); !errors.Is(err, injected) {
		t.Fatalf("lost failure: %v", err)
	}
	if !mouseDown {
		t.Fatal("test did not leave button pending")
	}
	blocked = false
	if err := injector.Close(); err != nil || mouseDown {
		t.Fatalf("close error=%v button down=%v", err, mouseDown)
	}
	if err := injector.Apply(protocol.Click{Button: protocol.ButtonRight}); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("closed injector accepted a command: %v", err)
	}
	if ready, _ := injector.Ready(); ready {
		t.Fatal("closed SendInput reports ready")
	}
}
