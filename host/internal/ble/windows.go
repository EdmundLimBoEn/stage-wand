//go:build windows

package ble

import (
	"errors"
	"fmt"
	"log"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/EdmundLimBoEn/stage-wand/host/internal/protocol"
	"github.com/go-ole/go-ole"
	"github.com/saltosystems/winrt-go"
	winbt "github.com/saltosystems/winrt-go/windows/devices/bluetooth"
	"github.com/saltosystems/winrt-go/windows/devices/bluetooth/genericattributeprofile"
	"github.com/saltosystems/winrt-go/windows/foundation"
	"github.com/saltosystems/winrt-go/windows/storage/streams"
)

const (
	windowsAsyncTimeout     = 5 * time.Second
	windowsAdvertisingRetry = 5 * time.Second
)

type windowsPeripheral struct {
	stop                 chan struct{}
	done                 chan struct{}
	closeOnce            sync.Once
	closeErr             error
	mu                   sync.Mutex
	closing              bool
	advertising          bool
	note                 string
	nextAdvertisingRetry time.Time
	callbacks            sync.WaitGroup
	session              *Session
	provider             *genericattributeprofile.GattServiceProvider
	command              *genericattributeprofile.GattLocalCharacteristic
	reply                *genericattributeprofile.GattLocalCharacteristic
	writeH               *foundation.TypedEventHandler
	subH                 *foundation.TypedEventHandler
	writeToken           foundation.EventRegistrationToken
	subToken             foundation.EventRegistrationToken
	writeAdded           bool
	subAdded             bool
}

func Start(hooks Hooks, localName string) Peripheral {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		return Unavailable(errors.New("Bluetooth requires 64-bit Windows"))
	}
	type result struct {
		peripheral *windowsPeripheral
		err        error
	}
	ready := make(chan result)
	go func() {
		leave, err := enterWinRT()
		if err != nil {
			ready <- result{err: err}
			return
		}
		defer leave()
		p, err := startWindows(hooks)
		if err != nil {
			ready <- result{err: err}
			return
		}
		ready <- result{peripheral: p}
		// Supervision and teardown share the creator's apartment and never race native release.
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-p.stop:
				p.closeErr = p.shutdown()
				close(p.done)
				return
			case now := <-ticker.C:
				status, err := p.provider.GetAdvertisementStatus()
				if p.updateAdvertising(status, err, now) {
					_ = p.provider.StopAdvertising()
					if err := p.advertise(); err != nil {
						p.mu.Lock()
						p.note = fmt.Sprintf("Bluetooth unavailable: %v; retrying", err)
						p.mu.Unlock()
					}
				}
			}
		}
	}()
	started := <-ready
	if started.err != nil {
		return Unavailable(started.err)
	}
	return started.peripheral
}

func enterWinRT() (func(), error) {
	runtime.LockOSThread()
	err := ole.RoInitialize(1)
	if err != nil {
		var result *ole.OleError
		if !errors.As(err, &result) || result.Code() != 1 {
			runtime.UnlockOSThread()
			return nil, err
		}
	}
	return func() {
		syscall.NewLazyDLL("combase.dll").NewProc("RoUninitialize").Call()
		runtime.UnlockOSThread()
	}, nil
}

func startWindows(hooks Hooks) (_ *windowsPeripheral, err error) {
	p := &windowsPeripheral{stop: make(chan struct{}), done: make(chan struct{})}
	defer func() {
		if err != nil {
			_ = p.shutdown()
		}
	}()
	op, err := createGattProvider(guid(ServiceUUID))
	if err != nil {
		return nil, err
	}
	defer op.Release()
	if err := awaitAsync(op); err != nil {
		return nil, err
	}
	res, err := op.GetResults()
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, errors.New("no GATT service provider result")
	}
	result := (*genericattributeprofile.GattServiceProviderResult)(res)
	defer result.Release()
	if code, err := result.GetError(); err != nil {
		return nil, err
	} else if code != winbt.BluetoothErrorSuccess {
		return nil, fmt.Errorf("GATT service provider error %d", code)
	}
	p.provider, err = result.GetServiceProvider()
	if err != nil {
		return nil, err
	}
	if p.provider == nil {
		return nil, errors.New("no GATT service provider")
	}
	service, err := p.provider.GetService()
	if err != nil {
		return nil, err
	}
	if service == nil {
		return nil, errors.New("no GATT service")
	}
	defer service.Release()
	hooks.OnActions = p.apply
	p.session = NewSession(hooks)
	p.command, err = p.addChar(service, CommandUUID, genericattributeprofile.GattCharacteristicPropertiesWriteWithoutResponse|genericattributeprofile.GattCharacteristicPropertiesWrite)
	if err != nil {
		return nil, err
	}
	writeGUID := winrt.ParameterizedInstanceGUID(
		foundation.GUIDTypedEventHandler,
		genericattributeprofile.SignatureGattLocalCharacteristic,
		genericattributeprofile.SignatureGattWriteRequestedEventArgs,
	)
	p.writeH = newTypedEventHandler(ole.NewGUID(writeGUID), func(_ *foundation.TypedEventHandler, _, args unsafe.Pointer) {
		p.onWrite((*genericattributeprofile.GattWriteRequestedEventArgs)(args))
	})
	p.writeToken, err = p.command.AddWriteRequested(p.writeH)
	if err != nil {
		return nil, err
	}
	p.writeAdded = true
	p.reply, err = p.addChar(service, ReplyUUID, genericattributeprofile.GattCharacteristicPropertiesNotify)
	if err != nil {
		return nil, err
	}
	subGUID := winrt.ParameterizedInstanceGUID(
		foundation.GUIDTypedEventHandler,
		genericattributeprofile.SignatureGattLocalCharacteristic,
		"cinterface(IInspectable)",
	)
	p.subH = newTypedEventHandler(ole.NewGUID(subGUID), func(_ *foundation.TypedEventHandler, _, _ unsafe.Pointer) {
		p.pruneSubscribers()
	})
	p.subToken, err = p.reply.AddSubscribedClientsChanged(p.subH)
	if err != nil {
		return nil, err
	}
	p.subAdded = true
	if err := p.advertise(); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(windowsAsyncTimeout)
	for {
		status, err := p.provider.GetAdvertisementStatus()
		if err != nil {
			return nil, err
		}
		switch status {
		case genericattributeprofile.GattServiceProviderAdvertisementStatusStarted:
			p.updateAdvertising(status, nil, time.Now())
			return p, nil
		case genericattributeprofile.GattServiceProviderAdvertisementStatusStartedWithoutAllAdvertisementData:
			return nil, errors.New("Bluetooth advertisement could not include all requested data; stop other BLE advertisers and retry")
		case genericattributeprofile.GattServiceProviderAdvertisementStatusAborted,
			genericattributeprofile.GattServiceProviderAdvertisementStatusStopped:
			return nil, fmt.Errorf("GATT advertising failed with status %d", status)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("GATT advertising did not start (status %d)", status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (p *windowsPeripheral) begin() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closing {
		return false
	}
	p.callbacks.Add(1)
	return true
}

func (p *windowsPeripheral) Note() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.note
}

func (p *windowsPeripheral) isAdvertising() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.advertising && !p.closing
}

func (p *windowsPeripheral) advertise() error {
	params, err := genericattributeprofile.NewGattServiceProviderAdvertisingParameters()
	if err != nil {
		return err
	}
	defer params.Release()
	if err := params.SetIsConnectable(true); err != nil {
		return err
	}
	if err := params.SetIsDiscoverable(true); err != nil {
		return err
	}
	return p.provider.StartAdvertisingWithParameters(params)
}

func (p *windowsPeripheral) updateAdvertising(status genericattributeprofile.GattServiceProviderAdvertisementStatus, statusErr error, now time.Time) bool {
	p.mu.Lock()
	if p.closing {
		p.mu.Unlock()
		return false
	}
	if statusErr == nil && status == genericattributeprofile.GattServiceProviderAdvertisementStatusStarted {
		p.advertising = true
		p.note = "advertising " + ServiceUUID
		p.nextAdvertisingRetry = time.Time{}
		p.mu.Unlock()
		return false
	}
	p.advertising = false
	if statusErr == nil {
		statusErr = fmt.Errorf("GATT advertising is unavailable (status %d)", status)
	}
	p.note = fmt.Sprintf("Bluetooth unavailable: %v; retrying", statusErr)
	retry := !now.Before(p.nextAdvertisingRetry)
	if retry {
		p.nextAdvertisingRetry = now.Add(windowsAdvertisingRetry)
	}
	p.mu.Unlock()
	p.session.Drop(p.session.Authed())
	return retry
}

func (p *windowsPeripheral) Kick() {
	p.session.Kick()
}

func (p *windowsPeripheral) Close() error {
	p.closeOnce.Do(func() { close(p.stop) })
	<-p.done
	return p.closeErr
}

func (p *windowsPeripheral) shutdown() error {
	p.mu.Lock()
	p.closing = true
	p.advertising = false
	p.note = "Bluetooth stopped"
	p.mu.Unlock()
	if p.session != nil {
		p.session.Close()
	}
	p.callbacks.Wait()
	var errs []error
	if p.provider != nil {
		errs = append(errs, p.provider.StopAdvertising())
	}
	if p.writeAdded {
		errs = append(errs, removeCharacteristicEvent(p.command, 15, p.writeToken))
	}
	if p.subAdded {
		errs = append(errs, removeCharacteristicEvent(p.reply, 11, p.subToken))
	}
	if p.writeH != nil {
		p.writeH.IUnknown.Release()
	}
	if p.subH != nil {
		p.subH.IUnknown.Release()
	}
	if p.command != nil {
		p.command.Release()
	}
	if p.reply != nil {
		p.reply.Release()
	}
	if p.provider != nil {
		p.provider.Release()
	}
	return errors.Join(errs...)
}

func (p *windowsPeripheral) addChar(service *genericattributeprofile.GattLocalService, uuid string, props genericattributeprofile.GattCharacteristicProperties) (*genericattributeprofile.GattLocalCharacteristic, error) {
	params, err := genericattributeprofile.NewGattLocalCharacteristicParameters()
	if err != nil {
		return nil, err
	}
	defer params.Release()
	if err := params.SetCharacteristicProperties(props); err != nil {
		return nil, err
	}
	op, err := createGattCharacteristic(service, guid(uuid), params)
	if err != nil {
		return nil, err
	}
	if op == nil {
		return nil, errors.New("no characteristic creation operation")
	}
	defer op.Release()
	if err := awaitAsync(op); err != nil {
		return nil, err
	}
	res, err := op.GetResults()
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, errors.New("no characteristic creation result")
	}
	result := (*genericattributeprofile.GattLocalCharacteristicResult)(res)
	defer result.Release()
	if code, err := result.GetError(); err != nil {
		return nil, err
	} else if code != winbt.BluetoothErrorSuccess {
		return nil, fmt.Errorf("GATT characteristic error %d", code)
	}
	char, err := result.GetCharacteristic()
	if err == nil && char == nil {
		err = errors.New("no GATT characteristic")
	}
	return char, err
}

func (p *windowsPeripheral) onWrite(args *genericattributeprofile.GattWriteRequestedEventArgs) {
	if args == nil || !p.begin() {
		return
	}
	defer p.callbacks.Done()
	if !p.isAdvertising() {
		return
	}
	leave, err := enterWinRT()
	if err != nil {
		return
	}
	defer leave()
	deferral, err := args.GetDeferral()
	if err != nil || deferral == nil {
		return
	}
	defer deferral.Release()
	defer deferral.Complete()
	session, err := args.GetSession()
	if err != nil || session == nil {
		return
	}
	defer session.Release()
	clientID, err := gattSessionDeviceID(session)
	if err != nil || clientID == "" {
		return
	}
	op, err := args.GetRequestAsync()
	if err != nil || op == nil {
		return
	}
	defer op.Release()
	if err := awaitAsync(op); err != nil {
		return
	}
	res, err := op.GetResults()
	if err != nil || res == nil {
		return
	}
	req := (*genericattributeprofile.GattWriteRequest)(res)
	defer req.Release()
	option, err := req.GetOption()
	if err != nil {
		return
	}
	reject := func(code uint8) {
		if option == genericattributeprofile.GattWriteOptionWriteWithResponse {
			_ = req.RespondWithProtocolError(code)
		}
	}
	offset, err := req.GetOffset()
	if err != nil || offset != 0 {
		reject(0x07) // ATT Invalid Offset: every command is a complete JSON frame.
		return
	}
	buf, err := req.GetValue()
	if err != nil || buf == nil {
		reject(0x0d)
		return
	}
	defer buf.Release()
	payload, err := bufferToSlice(buf)
	if err != nil {
		reject(0x0d) // ATT Invalid Attribute Value Length.
		return
	}
	command, err := protocol.ParseCommand(payload)
	if err != nil {
		reject(0x0e)
		return
	}
	client := p.subscribed(clientID)
	if client == nil {
		reject(0x03)
		return
	}
	limit, err := client.GetMaxNotificationSize()
	client.Release()
	if err != nil || limit < protocol.MinimumCommandBytes {
		reject(0x0d)
		return
	}
	if option == genericattributeprofile.GattWriteOptionWriteWithResponse {
		if err := req.Respond(); err != nil {
			return
		}
	}
	if !p.isAdvertising() {
		return
	}
	p.session.Handle(clientID, payload)
	if _, isAuth := command.(protocol.Auth); isAuth {
		// An unsubscribe can race authentication before an owner exists to prune.
		client := p.subscribed(clientID)
		if client == nil || !p.isAdvertising() {
			p.session.Drop(clientID)
		}
		if client != nil {
			client.Release()
		}
	}
}

func (p *windowsPeripheral) apply(actions []Action) {
	if !p.begin() {
		return
	}
	defer p.callbacks.Done()
	leave, err := enterWinRT()
	if err != nil {
		return
	}
	defer leave()
	for _, action := range actions {
		if len(action.Notify) > 0 && action.Client != "" {
			if err := p.notifyClient(action.Client, action.Notify); err != nil {
				log.Printf("Bluetooth reply failed: %v", err)
			}
		}
		if action.Drop != "" && action.Reason != "" {
			if err := p.notifyClient(action.Drop, mustReply(protocol.Bye{Reason: action.Reason})); err != nil {
				log.Printf("Bluetooth disconnect reply failed: %v", err)
			}
		}
	}
}

func (p *windowsPeripheral) notifyClient(id string, payload []byte) error {
	client := p.subscribed(id)
	if client == nil {
		return errors.New("client is not subscribed")
	}
	defer client.Release()
	limit, err := client.GetMaxNotificationSize()
	if err != nil {
		return err
	}
	if len(payload) > int(limit) {
		return fmt.Errorf("reply exceeds client notification limit of %d bytes", limit)
	}
	buf, err := sliceToBuffer(payload)
	if err != nil {
		return err
	}
	defer buf.Release()
	op, err := p.reply.NotifyValueForSubscribedClientAsync(buf, client)
	if err != nil {
		return err
	}
	if op == nil {
		return errors.New("no notification operation")
	}
	defer op.Release()
	if err := awaitAsync(op); err != nil {
		return err
	}
	res, err := op.GetResults()
	if err != nil {
		return err
	}
	if res == nil {
		return errors.New("no notification result")
	}
	result := (*genericattributeprofile.GattClientNotificationResult)(res)
	defer result.Release()
	status, err := result.GetStatus()
	if err != nil {
		return err
	}
	if status != genericattributeprofile.GattCommunicationStatusSuccess {
		return fmt.Errorf("GATT notification failed with status %d", status)
	}
	return nil
}

func (p *windowsPeripheral) subscribed(id string) *genericattributeprofile.GattSubscribedClient {
	if p.reply == nil || id == "" {
		return nil
	}
	vec, err := p.reply.GetSubscribedClients()
	if err != nil || vec == nil {
		return nil
	}
	defer vec.Release()
	size, err := vec.GetSize()
	if err != nil {
		return nil
	}
	for i := uint32(0); i < size; i++ {
		element, err := vec.GetAt(i)
		if err != nil || element == nil {
			continue
		}
		client := (*genericattributeprofile.GattSubscribedClient)(element)
		session, err := client.GetSession()
		if err == nil && session != nil {
			got, err := gattSessionDeviceID(session)
			session.Release()
			if err == nil && got == id {
				return client
			}
		}
		client.Release()
	}
	return nil
}

func (p *windowsPeripheral) pruneSubscribers() {
	if !p.begin() {
		return
	}
	defer p.callbacks.Done()
	leave, err := enterWinRT()
	if err != nil {
		return
	}
	defer leave()
	authed := p.session.Authed()
	if authed == "" {
		return
	}
	client := p.subscribed(authed)
	if client == nil {
		p.session.Drop(authed)
		return
	}
	client.Release()
}

func guid(uuid string) syscall.GUID {
	g := ole.NewGUID(uuid)
	return syscall.GUID{Data1: g.Data1, Data2: g.Data2, Data3: g.Data3, Data4: g.Data4}
}

type asyncInfoVtbl struct {
	ole.IInspectableVtbl
	GetID        uintptr
	GetStatus    uintptr
	GetErrorCode uintptr
	Cancel       uintptr
	Close        uintptr
}

func awaitAsync(op *foundation.IAsyncOperation) error {
	return awaitAsyncTimeout(op, windowsAsyncTimeout)
}

func awaitAsyncTimeout(op *foundation.IAsyncOperation, timeout time.Duration) error {
	if op == nil {
		return errors.New("nil async operation")
	}
	info, err := op.QueryInterface(ole.NewGUID("00000036-0000-0000-c000-000000000046"))
	if err != nil {
		return err
	}
	defer info.Release()
	vt := (*asyncInfoVtbl)(unsafe.Pointer(info.RawVTable))
	deadline := time.Now().Add(timeout)
	for {
		var status foundation.AsyncStatus
		hr, _, _ := syscall.SyscallN(vt.GetStatus, uintptr(unsafe.Pointer(info)), uintptr(unsafe.Pointer(&status)))
		if int32(hr) < 0 {
			return ole.NewError(hr)
		}
		switch status {
		case foundation.AsyncStatusCompleted:
			return nil
		case foundation.AsyncStatusCanceled:
			return errors.New("GATT operation canceled")
		case foundation.AsyncStatusError:
			var code int32
			hr, _, _ = syscall.SyscallN(vt.GetErrorCode, uintptr(unsafe.Pointer(info)), uintptr(unsafe.Pointer(&code)))
			if int32(hr) < 0 {
				return ole.NewError(hr)
			}
			return fmt.Errorf("GATT operation failed: HRESULT 0x%08x", uint32(code))
		case foundation.AsyncStatusStarted:
		default:
			return fmt.Errorf("unknown GATT operation status %d", status)
		}
		if !time.Now().Before(deadline) {
			_, _, _ = syscall.SyscallN(vt.Cancel, uintptr(unsafe.Pointer(info)))
			return errors.New("GATT operation timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func bufferToSlice(buffer *streams.IBuffer) ([]byte, error) {
	if buffer == nil {
		return nil, errors.New("nil GATT buffer")
	}
	n, err := buffer.GetLength()
	if err != nil {
		return nil, err
	}
	if n == 0 || n > protocol.MaxFrameBytes {
		return nil, errors.New("invalid GATT frame length")
	}
	access, err := buffer.QueryInterface(ole.NewGUID("905a0fef-bc53-11df-8c49-001e4fc686da"))
	if err != nil {
		return nil, err
	}
	defer access.Release()
	type bufferAccessVtbl struct {
		ole.IUnknownVtbl
		Buffer uintptr
	}
	vt := (*bufferAccessVtbl)(unsafe.Pointer(access.RawVTable))
	var data *byte
	hr, _, _ := syscall.SyscallN(vt.Buffer, uintptr(unsafe.Pointer(access)), uintptr(unsafe.Pointer(&data)))
	if int32(hr) < 0 {
		return nil, ole.NewError(hr)
	}
	if data == nil {
		return nil, errors.New("nil GATT buffer data")
	}
	return append([]byte(nil), unsafe.Slice(data, int(n))...), nil
}

func sliceToBuffer(payload []byte) (*streams.IBuffer, error) {
	if len(payload) == 0 || len(payload) > protocol.MaxFrameBytes {
		return nil, errors.New("invalid GATT frame length")
	}
	writer, err := streams.NewDataWriter()
	if err != nil {
		return nil, err
	}
	defer writer.Release()
	defer writer.Close()
	if err := writer.WriteBytes(uint32(len(payload)), payload); err != nil {
		return nil, err
	}
	return writer.DetachBuffer()
}

func gattSessionDeviceID(session *genericattributeprofile.GattSession) (string, error) {
	if session == nil {
		return "", errors.New("nil session")
	}
	itf, err := session.QueryInterface(ole.NewGUID("d23b5143-e04e-4c24-999c-9c256f9856b1"))
	if err != nil {
		return "", err
	}
	defer itf.Release()
	type vtbl struct {
		ole.IInspectableVtbl
		GetDeviceID uintptr
	}
	vt := (*vtbl)(unsafe.Pointer(itf.RawVTable))
	var out *winbt.BluetoothDeviceId
	hr, _, _ := syscall.SyscallN(vt.GetDeviceID, uintptr(unsafe.Pointer(itf)), uintptr(unsafe.Pointer(&out)))
	if int32(hr) < 0 {
		return "", ole.NewError(hr)
	}
	if out == nil {
		return "", errors.New("nil device id")
	}
	defer out.Release()
	return out.GetId()
}

func createGattProvider(id syscall.GUID) (*foundation.IAsyncOperation, error) {
	factory, err := ole.RoGetActivationFactory("Windows.Devices.Bluetooth.GenericAttributeProfile.GattServiceProvider", ole.NewGUID("31794063-5256-4054-a4f4-7bbe7755a57e"))
	if err != nil {
		return nil, err
	}
	defer factory.Release()
	type factoryVtbl struct {
		ole.IInspectableVtbl
		CreateAsync uintptr
	}
	vt := (*factoryVtbl)(unsafe.Pointer(factory.RawVTable))
	var op *foundation.IAsyncOperation
	// winrt-go's generated static wrapper passes null instead of the factory's COM this pointer.
	var hr uintptr
	if runtime.GOARCH == "arm64" {
		words := *(*[2]uintptr)(unsafe.Pointer(&id))
		hr, _, _ = syscall.SyscallN(vt.CreateAsync, uintptr(unsafe.Pointer(factory)), words[0], words[1], uintptr(unsafe.Pointer(&op)))
	} else {
		hr, _, _ = syscall.SyscallN(vt.CreateAsync, uintptr(unsafe.Pointer(factory)), uintptr(unsafe.Pointer(&id)), uintptr(unsafe.Pointer(&op)))
	}
	if int32(hr) < 0 {
		return nil, ole.NewError(hr)
	}
	if op == nil {
		return nil, errors.New("no GATT service provider operation")
	}
	return op, nil
}

func removeCharacteristicEvent(char *genericattributeprofile.GattLocalCharacteristic, method int, token foundation.EventRegistrationToken) error {
	itf, err := char.QueryInterface(ole.NewGUID("aede376d-5412-4d74-92a8-8deb8526829c"))
	if err != nil {
		return err
	}
	defer itf.Release()
	type characteristicVtbl struct {
		ole.IInspectableVtbl
		Methods [18]uintptr
	}
	vt := (*characteristicVtbl)(unsafe.Pointer(itf.RawVTable))
	// EventRegistrationToken is an eight-byte value, not a pointer, in the 64-bit WinRT ABI.
	hr, _, _ := syscall.SyscallN(vt.Methods[method], uintptr(unsafe.Pointer(itf)), uintptr(token.Value))
	if int32(hr) < 0 {
		return ole.NewError(hr)
	}
	return nil
}

func createGattCharacteristic(service *genericattributeprofile.GattLocalService, id syscall.GUID, params *genericattributeprofile.GattLocalCharacteristicParameters) (*foundation.IAsyncOperation, error) {
	itf, err := service.QueryInterface(ole.NewGUID("f513e258-f7f7-4902-b803-57fcc7d6fe83"))
	if err != nil {
		return nil, err
	}
	defer itf.Release()
	type serviceVtbl struct {
		ole.IInspectableVtbl
		GetUuid                   uintptr
		CreateCharacteristicAsync uintptr
	}
	vt := (*serviceVtbl)(unsafe.Pointer(itf.RawVTable))
	var op *foundation.IAsyncOperation
	var hr uintptr
	// A 16-byte GUID is indirect on AMD64 but occupies two registers on ARM64.
	if runtime.GOARCH == "arm64" {
		words := *(*[2]uintptr)(unsafe.Pointer(&id))
		hr, _, _ = syscall.SyscallN(vt.CreateCharacteristicAsync, uintptr(unsafe.Pointer(itf)), words[0], words[1], uintptr(unsafe.Pointer(params)), uintptr(unsafe.Pointer(&op)))
	} else {
		hr, _, _ = syscall.SyscallN(vt.CreateCharacteristicAsync, uintptr(unsafe.Pointer(itf)), uintptr(unsafe.Pointer(&id)), uintptr(unsafe.Pointer(params)), uintptr(unsafe.Pointer(&op)))
	}
	if int32(hr) < 0 {
		return nil, ole.NewError(hr)
	}
	return op, nil
}

var typedHandlerQueryInterface = syscall.NewCallback(func(handler *foundation.TypedEventHandler, iid *ole.GUID, out *unsafe.Pointer) uintptr {
	if out == nil || iid == nil {
		return ole.E_POINTER
	}
	*out = nil
	if !ole.IsEqualGUID(iid, ole.IID_IUnknown) && !ole.IsEqualGUID(iid, handler.GetIID()) {
		return ole.E_NOINTERFACE
	}
	handler.IUnknown.AddRef()
	*out = unsafe.Pointer(handler)
	return ole.S_OK
})

func newTypedEventHandler(iid *ole.GUID, callback foundation.TypedEventHandlerCallback) *foundation.TypedEventHandler {
	handler := foundation.NewTypedEventHandler(iid, callback)
	// The generated delegate advertises IInspectable without implementing its vtable.
	handler.IUnknown.VTable().QueryInterface = typedHandlerQueryInterface
	return handler
}
