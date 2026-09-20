//go:build windows

package input

import (
	"fmt"
	"unsafe"

	"github.com/EdmundLimBoEn/stage-wand/host/internal/protocol"
	"golang.org/x/sys/windows"
)

const (
	inputMouse     = 0
	inputKeyboard  = 1
	mouseMove      = 0x0001
	mouseLeftDown  = 0x0002
	mouseLeftUp    = 0x0004
	mouseRightDown = 0x0008
	mouseRightUp   = 0x0010
	mouseWheel     = 0x0800
	mouseHWheel    = 0x1000
	keyUp          = 0x0002
	vkLeft         = 0x25
	vkUp           = 0x26
	vkRight        = 0x27
	vkEscape       = 0x1B
	vkControl      = 0x11
	vkLWin         = 0x5B
	vkTab          = 0x09
)

var (
	user32        = windows.NewLazySystemDLL("user32.dll")
	procSendInput = user32.NewProc("SendInput")
)

type mouseData struct {
	Dx        int32
	Dy        int32
	MouseData uint32
	Flags     uint32
	Time      uint32
	Extra     uintptr
}

type keyboardData struct {
	Vk    uint16
	Scan  uint16
	Flags uint32
	Time  uint32
	Extra uintptr
}

type inputRecord struct {
	Type uint32
	Data mouseData
}

type SendInputInjector struct{}

func Diagnose() Probe {
	return Probe{Device: "SendInput", Exists: true, Writable: true, Hint: "SendInput ready"}
}

func Open() (Injector, error) {
	return SendInputInjector{}, nil
}

func OpenOrError() (Injector, error) { return Open() }

func (SendInputInjector) Ready() (bool, string) { return true, "SendInput ready" }
func (SendInputInjector) Close() error          { return nil }

func (SendInputInjector) Apply(command protocol.Command) error {
	switch v := command.(type) {
	case protocol.Auth:
		return nil
	case protocol.Move:
		dx, dy := relative(v.Dx, v.Dy)
		if dx == 0 && dy == 0 {
			return nil
		}
		return sendMouse(dx, dy, 0, mouseMove)
	case protocol.Click:
		down, up := uint32(mouseLeftDown), uint32(mouseLeftUp)
		if v.Button == protocol.ButtonRight {
			down, up = mouseRightDown, mouseRightUp
		}
		if err := sendMouse(0, 0, 0, down); err != nil {
			return err
		}
		return sendMouse(0, 0, 0, up)
	case protocol.Scroll:
		dx, dy := relative(v.Dx, v.Dy)
		if dy != 0 {
			if err := sendMouse(0, 0, uint32(dy), mouseWheel); err != nil {
				return err
			}
		}
		if dx != 0 {
			if err := sendMouse(0, 0, uint32(dx), mouseHWheel); err != nil {
				return err
			}
		}
		return nil
	case protocol.KeyPress:
		return tap(virtualKey(v.Key))
	case protocol.ChordPress:
		switch v.Chord {
		case protocol.SpaceLeft:
			return chord([]uint16{vkLWin, vkControl, vkLeft})
		case protocol.SpaceRight:
			return chord([]uint16{vkLWin, vkControl, vkRight})
		case protocol.MissionCtrl:
			return chord([]uint16{vkLWin, vkTab})
		}
	}
	return nil
}

func virtualKey(key protocol.Key) uint16 {
	switch key {
	case protocol.KeyLeft:
		return vkLeft
	case protocol.KeyRight:
		return vkRight
	default:
		return vkEscape
	}
}

func tap(vk uint16) error {
	if err := sendKey(vk, 0); err != nil {
		return err
	}
	return sendKey(vk, keyUp)
}

func chord(keys []uint16) error {
	for _, key := range keys {
		if err := sendKey(key, 0); err != nil {
			return err
		}
	}
	for i := len(keys) - 1; i >= 0; i-- {
		if err := sendKey(keys[i], keyUp); err != nil {
			return err
		}
	}
	return nil
}

func sendMouse(dx, dy int32, data, flags uint32) error {
	in := inputRecord{Type: inputMouse, Data: mouseData{Dx: dx, Dy: dy, MouseData: data, Flags: flags}}
	return send(unsafe.Pointer(&in), unsafe.Sizeof(in))
}

func sendKey(vk uint16, flags uint32) error {
	in := inputRecord{Type: inputKeyboard}
	// INPUT always reserves its full union, even for the smaller KEYBDINPUT member.
	*(*keyboardData)(unsafe.Pointer(&in.Data)) = keyboardData{Vk: vk, Flags: flags}
	return send(unsafe.Pointer(&in), unsafe.Sizeof(in))
}

func send(p unsafe.Pointer, size uintptr) error {
	n, _, err := procSendInput.Call(1, uintptr(p), size)
	if n == 0 {
		return fmt.Errorf("SendInput: %v", err)
	}
	return nil
}
