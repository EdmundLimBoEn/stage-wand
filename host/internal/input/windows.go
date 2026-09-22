//go:build windows

package input

import (
	"fmt"
	"os"
	"sync"
	"syscall"
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
	buttonLeft     = 0x100
	buttonRight    = 0x101
	keyExtended    = 0x0001
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

type SendInputInjector struct {
	mu        sync.Mutex
	keys      keyState
	closed    bool
	lastErr   error
	sendEvent func(inputRecord) error
}

func Diagnose() Probe {
	return Probe{Device: "SendInput", Exists: true, Writable: true, Hint: "SendInput ready"}
}

func Open() (Injector, error) {
	return &SendInputInjector{}, nil
}

func OpenOrError() (Injector, error) { return Open() }

func (s *SendInputInjector) Ready() (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false, "input device closed"
	}
	if s.lastErr != nil {
		return false, s.lastErr.Error()
	}
	return true, "SendInput ready"
}

func (s *SendInputInjector) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.keys.release(s.emitKey)
	s.closed = true
	return err
}

func (s *SendInputInjector) Apply(command protocol.Command) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return os.ErrClosed
	}
	if _, ok := command.(protocol.Auth); ok {
		return nil
	}
	defer func() {
		if err != nil {
			s.lastErr = err
		}
	}()
	if err := s.keys.release(s.emitKey); err != nil {
		return err
	}
	switch v := command.(type) {
	case protocol.Move:
		dx, dy := relative(v.Dx, v.Dy)
		if dx == 0 && dy == 0 {
			return nil
		}
		return s.sendMouse(dx, dy, 0, mouseMove)
	case protocol.Click:
		button := uint16(buttonLeft)
		if v.Button == protocol.ButtonRight {
			button = buttonRight
		}
		return s.keys.tap([]uint16{button}, s.emitKey)
	case protocol.Scroll:
		dx, dy := relative(v.Dx, v.Dy)
		if dy != 0 {
			if err := s.sendMouse(0, 0, uint32(dy), mouseWheel); err != nil {
				return err
			}
		}
		if dx != 0 {
			if err := s.sendMouse(0, 0, uint32(dx), mouseHWheel); err != nil {
				return err
			}
		}
		return nil
	case protocol.KeyPress:
		return s.keys.tap([]uint16{virtualKey(v.Key)}, s.emitKey)
	case protocol.ChordPress:
		switch v.Chord {
		case protocol.SpaceLeft:
			return s.keys.tap([]uint16{vkLWin, vkControl, vkLeft}, s.emitKey)
		case protocol.SpaceRight:
			return s.keys.tap([]uint16{vkLWin, vkControl, vkRight}, s.emitKey)
		case protocol.MissionCtrl:
			return s.keys.tap([]uint16{vkLWin, vkTab}, s.emitKey)
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

func (s *SendInputInjector) sendMouse(dx, dy int32, data, flags uint32) error {
	in := inputRecord{Type: inputMouse, Data: mouseData{Dx: dx, Dy: dy, MouseData: data, Flags: flags}}
	return s.send(in)
}

func (s *SendInputInjector) emitKey(vk uint16, down bool) error {
	if vk == buttonLeft || vk == buttonRight {
		downFlag, upFlag := uint32(mouseLeftDown), uint32(mouseLeftUp)
		if vk == buttonRight {
			downFlag, upFlag = mouseRightDown, mouseRightUp
		}
		if down {
			return s.sendMouse(0, 0, 0, downFlag)
		}
		return s.sendMouse(0, 0, 0, upFlag)
	}
	var flags uint32
	if !down {
		flags |= keyUp
	}
	if vk == vkLeft || vk == vkRight || vk == vkUp || vk == vkLWin {
		flags |= keyExtended
	}
	in := inputRecord{Type: inputKeyboard}
	// INPUT always reserves its full union, even for the smaller KEYBDINPUT member.
	*(*keyboardData)(unsafe.Pointer(&in.Data)) = keyboardData{Vk: vk, Flags: flags}
	return s.send(in)
}

func (s *SendInputInjector) send(in inputRecord) (err error) {
	defer func() {
		if err == nil {
			s.lastErr = nil
		}
	}()
	if s.sendEvent != nil {
		return s.sendEvent(in)
	}
	n, _, err := procSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
	if n == 0 {
		if err != nil && err != syscall.Errno(0) {
			return fmt.Errorf("SendInput: %w", err)
		}
		return fmt.Errorf("SendInput was blocked; input cannot control an elevated application or the secure desktop")
	}
	return nil
}
