//go:build windows

package input

import (
	"testing"
	"unsafe"
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
