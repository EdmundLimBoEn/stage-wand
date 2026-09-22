//go:build linux

package input

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
	"unsafe"

	"github.com/EdmundLimBoEn/stage-wand/host/internal/protocol"
	"golang.org/x/sys/unix"
)

const (
	busVirtual        = 0x06
	uinputMaxNameSize = 80
	absCnt            = 64
	evSyn             = 0x00
	evKey             = 0x01
	evRel             = 0x02
	synReport         = 0
	relX              = 0x00
	relY              = 0x01
	relHWheel         = 0x06
	relWheel          = 0x08
	relHWheelHiRes    = 0x0c
	relWheelHiRes     = 0x0b
	btnLeft           = 0x110
	btnRight          = 0x111
	keyEsc            = 1
	keyLeftCtrl       = 29
	keyUp             = 103
	keyLeft           = 105
	keyRight          = 106
	uiDevCreate       = 0x5501
	uiDevDestroy      = 0x5502
	uiSetEvbit        = 0x40045564
	uiSetKeybit       = 0x40045565
	uiSetRelbit       = 0x40045566
)

type inputID struct {
	Bustype uint16
	Vendor  uint16
	Product uint16
	Version uint16
}

type uinputUserDev struct {
	Name    [uinputMaxNameSize]byte
	ID      inputID
	FFMax   uint32
	AbsMax  [absCnt]int32
	AbsMin  [absCnt]int32
	AbsFuzz [absCnt]int32
	AbsFlat [absCnt]int32
}

type inputEvent struct {
	Time  unix.Timeval
	Type  uint16
	Code  uint16
	Value int32
}

type UInput struct {
	mu   sync.Mutex
	file interface {
		Write([]byte) (int, error)
		Close() error
		Fd() uintptr
	}
	probe          Probe
	keys           keyState
	wheelX, wheelY int64
	lastErr        error
}

var afterCreate = 100 * time.Millisecond

func Diagnose() Probe {
	kind, desktop, display := SessionFromEnv(os.Getenv)
	p := Probe{
		Device:  "/dev/uinput",
		Session: kind,
		Desktop: desktop,
		Display: display,
	}
	_, err := os.Stat(p.Device)
	if err != nil {
		p.Exists = false
		if !os.IsNotExist(err) {
			p.OpenErr = err.Error()
		}
		return FinishProbe(p)
	}
	p.Exists = true
	file, err := os.OpenFile(p.Device, os.O_WRONLY, 0)
	if err != nil {
		p.Writable = false
		p.OpenErr = err.Error()
		return FinishProbe(p)
	}
	file.Close()
	p.Writable = true
	return FinishProbe(p)
}

func Open() (Injector, error) {
	probe := Diagnose()
	file, err := os.OpenFile("/dev/uinput", os.O_WRONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("open /dev/uinput: %w (%s)", err, probe.Hint)
	}
	device := &UInput{file: file, probe: probe}
	if err := device.setup(); err != nil {
		file.Close()
		return nil, err
	}
	device.probe.Writable = true
	device.probe = FinishProbe(device.probe)
	return device, nil
}

func (u *UInput) setup() error {
	fd := int(u.file.Fd())
	if err := ioctl(fd, uiSetEvbit, evKey); err != nil {
		return err
	}
	if err := ioctl(fd, uiSetEvbit, evRel); err != nil {
		return err
	}
	if err := ioctl(fd, uiSetEvbit, evSyn); err != nil {
		return err
	}
	for _, code := range []int{btnLeft, btnRight, keyEsc, keyLeft, keyRight, keyUp, keyLeftCtrl} {
		if err := ioctl(fd, uiSetKeybit, code); err != nil {
			return err
		}
	}
	for _, code := range []int{relX, relY, relWheel, relHWheel, relWheelHiRes, relHWheelHiRes} {
		if err := ioctl(fd, uiSetRelbit, code); err != nil {
			return err
		}
	}
	var setup uinputUserDev
	copy(setup.Name[:], []byte("Stage Wand"))
	setup.ID = inputID{Bustype: busVirtual, Vendor: 0x1d6b, Product: 0x0001, Version: 1}
	if err := binary.Write(u.file, nativeEndian, setup); err != nil {
		return err
	}
	if err := ioctl(fd, uiDevCreate, 0); err != nil {
		return err
	}
	time.Sleep(afterCreate)
	return nil
}

func (u *UInput) Ready() (bool, string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.file == nil {
		return false, "input device closed"
	}
	if u.lastErr != nil {
		return false, u.lastErr.Error()
	}
	return u.probe.Ready()
}

func (u *UInput) Close() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.file == nil {
		return nil
	}
	err := u.keys.release(u.emitKey)
	ioctl(int(u.file.Fd()), uiDevDestroy, 0)
	err = errors.Join(err, u.file.Close())
	u.file = nil
	return err
}

func (u *UInput) Apply(command protocol.Command) (err error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.file == nil {
		return os.ErrClosed
	}
	if _, ok := command.(protocol.Auth); ok {
		return nil
	}
	defer func() {
		if err != nil {
			u.lastErr = err
		}
	}()
	if err := u.keys.release(u.emitKey); err != nil {
		return err
	}
	switch v := command.(type) {
	case protocol.Move:
		dx, dy := relative(v.Dx, v.Dy)
		if dx == 0 && dy == 0 {
			return nil
		}
		defer func() { err = u.flushAfterError(err) }()
		if dx != 0 {
			if err := u.emit(evRel, relX, dx); err != nil {
				return err
			}
		}
		if dy != 0 {
			if err := u.emit(evRel, relY, dy); err != nil {
				return err
			}
		}
		return u.emit(evSyn, synReport, 0)
	case protocol.Click:
		code := uint16(btnLeft)
		if v.Button == protocol.ButtonRight {
			code = btnRight
		}
		return u.keys.tap([]uint16{code}, u.emitKey)
	case protocol.Scroll:
		dx, dy := relative(v.Dx, v.Dy)
		if dx == 0 && dy == 0 {
			return nil
		}
		defer func() { err = u.flushAfterError(err) }()
		if dy != 0 {
			if err := u.emit(evRel, relWheelHiRes, dy); err != nil {
				return err
			}
			if err := u.emitWheel(relWheel, dy, &u.wheelY); err != nil {
				return err
			}
		}
		if dx != 0 {
			if err := u.emit(evRel, relHWheelHiRes, dx); err != nil {
				return err
			}
			if err := u.emitWheel(relHWheel, dx, &u.wheelX); err != nil {
				return err
			}
		}
		return u.emit(evSyn, synReport, 0)
	case protocol.KeyPress:
		return u.keys.tap([]uint16{keyCode(v.Key)}, u.emitKey)
	case protocol.ChordPress:
		code := uint16(keyLeft)
		switch v.Chord {
		case protocol.SpaceRight:
			code = keyRight
		case protocol.MissionCtrl:
			code = keyUp
		}
		return u.keys.tap([]uint16{keyLeftCtrl, code}, u.emitKey)
	default:
		return nil
	}
}

func keyCode(key protocol.Key) uint16 {
	switch key {
	case protocol.KeyLeft:
		return keyLeft
	case protocol.KeyRight:
		return keyRight
	default:
		return keyEsc
	}
}

func (u *UInput) emitKey(code uint16, down bool) error {
	var value int32
	if down {
		value = 1
	}
	if err := u.emit(evKey, code, value); err != nil {
		return err
	}
	return u.emit(evSyn, synReport, 0)
}

func (u *UInput) flushAfterError(err error) error {
	if err == nil {
		return nil
	}
	return errors.Join(err, u.emit(evSyn, synReport, 0))
}

func (u *UInput) emitWheel(code uint16, delta int32, remainder *int64) error {
	*remainder += int64(delta)
	steps := *remainder / 120
	if steps == 0 {
		return nil
	}
	if err := u.emit(evRel, code, int32(steps)); err != nil {
		return err
	}
	*remainder %= 120
	return nil
}

func (u *UInput) emit(evType, code uint16, value int32) error {
	event := inputEvent{Type: evType, Code: code, Value: value}
	err := binary.Write(u.file, nativeEndian, event)
	if err == nil {
		u.lastErr = nil
	}
	return err
}

func ioctl(fd int, request, arg int) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(request), uintptr(arg))
	if errno != 0 {
		return os.NewSyscallError("ioctl", errno)
	}
	return nil
}

var nativeEndian = func() binary.ByteOrder {
	buf := [2]byte{}
	*(*uint16)(unsafe.Pointer(&buf[0])) = 0x0102
	if buf[0] == 1 {
		return binary.BigEndian
	}
	return binary.LittleEndian
}()

func OpenOrError() (Injector, error) {
	probe := Diagnose()
	if !probe.Exists || !probe.Writable {
		return nil, fmt.Errorf("%s", probe.Hint)
	}
	return Open()
}
