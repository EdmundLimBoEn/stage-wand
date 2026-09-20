//go:build windows

package ble

import (
	"runtime"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWinRTInitializationStaysOnOneThread(t *testing.T) {
	leave, err := enterWinRT()
	if err != nil {
		t.Fatal(err)
	}
	defer leave()
	nestedLeave, err := enterWinRT()
	if err != nil {
		t.Fatalf("nested initialization: %v", err)
	}
	defer nestedLeave()
	thread := windows.GetCurrentThreadId()
	for i := 0; i < 100; i++ {
		runtime.Gosched()
		if current := windows.GetCurrentThreadId(); current != thread {
			t.Fatalf("moved from %d to %d", thread, current)
		}
	}
}
