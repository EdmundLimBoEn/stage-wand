//go:build windows

package ble

import (
	"bytes"
	"errors"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/EdmundLimBoEn/stage-wand/host/internal/control"
	"github.com/EdmundLimBoEn/stage-wand/host/internal/protocol"
	"github.com/go-ole/go-ole"
	"github.com/saltosystems/winrt-go/windows/devices/bluetooth/genericattributeprofile"
	"github.com/saltosystems/winrt-go/windows/foundation"
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

func TestWinRTBufferRoundTrip(t *testing.T) {
	leave, err := enterWinRT()
	if err != nil {
		t.Fatal(err)
	}
	defer leave()
	for _, payload := range [][]byte{[]byte(`{"t":"status"}`), bytes.Repeat([]byte("x"), protocol.MaxFrameBytes)} {
		buffer, err := sliceToBuffer(payload)
		if err != nil {
			t.Fatal(err)
		}
		got, err := bufferToSlice(buffer)
		buffer.Release()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("buffer round trip differs: got %d bytes, want %d", len(got), len(payload))
		}
	}
	for _, payload := range [][]byte{nil, bytes.Repeat([]byte("x"), protocol.MaxFrameBytes+1)} {
		if _, err := sliceToBuffer(payload); err == nil {
			t.Fatalf("accepted %d-byte payload", len(payload))
		}
	}
	if _, err := bufferToSlice(nil); err == nil {
		t.Fatal("accepted nil buffer")
	}
}

func TestTypedEventHandlerDoesNotAdvertiseUnsupportedInspectable(t *testing.T) {
	iid := ole.NewGUID("7e53ccf1-7a3b-45ad-b3d5-61b68f946900")
	handler := newTypedEventHandler(iid, func(_ *foundation.TypedEventHandler, _, _ unsafe.Pointer) {})
	defer handler.IUnknown.Release()
	for _, supported := range []*ole.GUID{iid, ole.IID_IUnknown} {
		itf, err := handler.IUnknown.QueryInterface(supported)
		if err != nil {
			t.Fatal(err)
		}
		itf.Release()
	}
	if itf, err := handler.IUnknown.QueryInterface(ole.IID_IInspectable); err == nil {
		itf.Release()
		t.Fatal("delegate advertised IInspectable without its vtable")
	}
}

type testAsyncOperation struct {
	foundation.IAsyncOperation
	info testAsyncInfo
}

type testAsyncInfo struct {
	ole.IInspectable
	status   foundation.AsyncStatus
	code     int32
	canceled bool
}

var testAsyncOperationVtable = foundation.IAsyncOperationVtbl{
	IInspectableVtbl: ole.IInspectableVtbl{IUnknownVtbl: ole.IUnknownVtbl{
		QueryInterface: syscall.NewCallback(func(op *testAsyncOperation, _ *ole.GUID, out *unsafe.Pointer) uintptr {
			*out = unsafe.Pointer(&op.info)
			return ole.S_OK
		}),
	}},
}

var testAsyncInfoVtable = asyncInfoVtbl{
	IInspectableVtbl: ole.IInspectableVtbl{IUnknownVtbl: ole.IUnknownVtbl{
		Release: syscall.NewCallback(func(_ unsafe.Pointer) uintptr { return 1 }),
	}},
	GetStatus: syscall.NewCallback(func(info *testAsyncInfo, out *foundation.AsyncStatus) uintptr {
		*out = info.status
		return ole.S_OK
	}),
	GetErrorCode: syscall.NewCallback(func(info *testAsyncInfo, out *int32) uintptr {
		*out = info.code
		return ole.S_OK
	}),
	Cancel: syscall.NewCallback(func(info *testAsyncInfo) uintptr {
		info.canceled = true
		return ole.S_OK
	}),
}

func TestAwaitAsyncHandlesCompletionFailureAndTimeout(t *testing.T) {
	for _, tt := range []struct {
		name       string
		status     foundation.AsyncStatus
		wantError  string
		wantCancel bool
	}{
		{"completed", foundation.AsyncStatusCompleted, "", false},
		{"failed", foundation.AsyncStatusError, "HRESULT 0x80004005", false},
		{"canceled", foundation.AsyncStatusCanceled, "canceled", false},
		{"timeout", foundation.AsyncStatusStarted, "timed out", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			op := &testAsyncOperation{info: testAsyncInfo{status: tt.status, code: -2147467259}}
			op.RawVTable = (*interface{})(unsafe.Pointer(&testAsyncOperationVtable))
			op.info.RawVTable = (*interface{})(unsafe.Pointer(&testAsyncInfoVtable))
			err := awaitAsyncTimeout(&op.IAsyncOperation, 0)
			if tt.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("got %v, want error containing %q", err, tt.wantError)
			}
			if op.info.canceled != tt.wantCancel {
				t.Fatalf("Cancel called = %v, want %v", op.info.canceled, tt.wantCancel)
			}
			runtime.KeepAlive(op)
		})
	}
	if err := awaitAsync(nil); err == nil {
		t.Fatal("accepted nil operation")
	}
}

func TestWindowsCloseWaitsForCallbacks(t *testing.T) {
	p := &windowsPeripheral{stop: make(chan struct{}), done: make(chan struct{})}
	if !p.begin() {
		t.Fatal("new peripheral rejected callback")
	}
	go func() {
		<-p.stop
		p.closeErr = p.shutdown()
		close(p.done)
	}()
	closed := make(chan error, 1)
	go func() { closed <- p.Close() }()
	deadline := time.After(time.Second)
	for {
		p.mu.Lock()
		closing := p.closing
		p.mu.Unlock()
		if closing {
			break
		}
		select {
		case <-deadline:
			t.Fatal("close did not begin")
		default:
			runtime.Gosched()
		}
	}
	if p.begin() {
		p.callbacks.Done()
		t.Fatal("accepted callback during shutdown")
	}
	select {
	case <-closed:
		t.Fatal("close returned with an active callback")
	default:
	}
	p.callbacks.Done()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("close did not finish after callback")
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsAdvertisingLossRevokesOwnerAndRecoveryRequiresAuth(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status genericattributeprofile.GattServiceProviderAdvertisementStatus
		err    error
	}{
		{"radio stopped", genericattributeprofile.GattServiceProviderAdvertisementStatusStopped, nil},
		{"advertising aborted", genericattributeprofile.GattServiceProviderAdvertisementStatusAborted, nil},
		{"incomplete advertisement", genericattributeprofile.GattServiceProviderAdvertisementStatusStartedWithoutAllAdvertisementData, nil},
		{"status lookup failed", genericattributeprofile.GattServiceProviderAdvertisementStatusStarted, errors.New("radio unavailable")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			commands := 0
			p := &windowsPeripheral{session: NewSession(Hooks{
				Control:   control.NewSession("0000"),
				OnCommand: func(protocol.Command) { commands++ },
			})}
			now := time.Unix(100, 0)
			started := genericattributeprofile.GattServiceProviderAdvertisementStatusStarted
			if p.updateAdvertising(started, nil, now) || !p.isAdvertising() {
				t.Fatal("healthy advertisement requested restart")
			}
			p.session.Handle("phone", []byte(`{"t":"auth","code":"0000"}`))
			if p.session.Authed() != "phone" {
				t.Fatal("initial auth failed")
			}
			if !p.updateAdvertising(tt.status, tt.err, now) {
				t.Fatal("first failure did not request restart")
			}
			if p.session.Authed() != "" || p.isAdvertising() {
				t.Fatal("advertising failure left a usable authenticated session")
			}
			if !strings.Contains(p.Note(), "unavailable") {
				t.Fatalf("misleading failure note: %q", p.Note())
			}
			if p.updateAdvertising(tt.status, tt.err, now.Add(time.Second)) {
				t.Fatal("restart ignored retry backoff")
			}
			if !p.updateAdvertising(tt.status, tt.err, now.Add(windowsAdvertisingRetry)) {
				t.Fatal("restart was not retried after backoff")
			}
			p.updateAdvertising(started, nil, now.Add(windowsAdvertisingRetry+time.Second))
			if !p.isAdvertising() || !strings.HasPrefix(p.Note(), "advertising ") {
				t.Fatalf("recovery not reflected: %q", p.Note())
			}
			p.session.Handle("phone", []byte(`{"t":"key","k":"right"}`))
			if commands != 0 {
				t.Fatal("recovery restored stale authentication")
			}
			p.session.Handle("phone", []byte(`{"t":"auth","code":"0000"}`))
			p.session.Handle("phone", []byte(`{"t":"key","k":"right"}`))
			if commands != 1 {
				t.Fatal("fresh authentication did not recover control")
			}
			if err := p.shutdown(); err != nil {
				t.Fatal(err)
			}
			if p.updateAdvertising(started, nil, now.Add(time.Minute)) || p.isAdvertising() || p.Note() != "Bluetooth stopped" {
				t.Fatal("late status update revived closed peripheral")
			}
		})
	}
}
