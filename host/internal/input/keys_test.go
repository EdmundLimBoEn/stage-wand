package input

import (
	"errors"
	"reflect"
	"testing"
)

func TestKeySequenceReleasesEveryPressedKeyAfterFailure(t *testing.T) {
	for failure := 1; failure <= 6; failure++ {
		t.Run(string(rune('0'+failure)), func(t *testing.T) {
			var state keyState
			calls := 0
			held := map[uint16]bool{}
			injected := errors.New("injection failed")
			err := state.tap([]uint16{1, 2, 3}, func(key uint16, down bool) error {
				calls++
				// Include a write that takes effect but fails its synchronization.
				held[key] = down
				if calls == failure {
					return injected
				}
				return nil
			})
			if !errors.Is(err, injected) {
				t.Fatalf("lost injection error: %v", err)
			}
			for key, down := range held {
				if down {
					t.Fatalf("key %d remains pressed after failure %d", key, failure)
				}
			}
			if len(state.held) != 0 {
				t.Fatalf("still held: %v", state.held)
			}
		})
	}
}

func TestKeySequenceRetainsFailedReleaseForRecovery(t *testing.T) {
	var state keyState
	injected := errors.New("device unavailable")
	err := state.tap([]uint16{1, 2}, func(key uint16, down bool) error {
		if !down && key == 2 {
			return injected
		}
		return nil
	})
	if !errors.Is(err, injected) || !reflect.DeepEqual(state.held, []uint16{2}) {
		t.Fatalf("error=%v pending=%v", err, state.held)
	}
	var events []int
	err = state.tap([]uint16{3}, func(key uint16, down bool) error {
		value := int(key)
		if !down {
			value = -value
		}
		events = append(events, value)
		return nil
	})
	if err != nil || !reflect.DeepEqual(events, []int{-2, 3, -3}) {
		t.Fatalf("error=%v events=%v", err, events)
	}
}
