//go:build windows

package ble

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/saltosystems/winrt-go"
	winbt "github.com/saltosystems/winrt-go/windows/devices/bluetooth"
	"github.com/saltosystems/winrt-go/windows/devices/bluetooth/genericattributeprofile"
	"github.com/saltosystems/winrt-go/windows/foundation"
	"github.com/saltosystems/winrt-go/windows/foundation/collections"
	"github.com/saltosystems/winrt-go/windows/storage/streams"
)

type windowsPeripheral struct {
	session  *Session
	provider *genericattributeprofile.GattServiceProvider
	reply    *genericattributeprofile.GattLocalCharacteristic
	writeH   *foundation.TypedEventHandler
	subH     *foundation.TypedEventHandler
}

func Start(hooks Hooks, localName string) Peripheral {
	p, err := startWindows(hooks)
	if err != nil {
		return Unavailable(err)
	}
	_ = localName
	return p
}

func startWindows(hooks Hooks) (*windowsPeripheral, error) {
	_ = ole.RoInitialize(1)
	op, err := genericattributeprofile.GattServiceProviderCreateAsync(guid(ServiceUUID))
	if err != nil {
		return nil, err
	}
	if err := awaitAsync(op, genericattributeprofile.SignatureGattServiceProviderResult); err != nil {
		return nil, err
	}
	res, err := op.GetResults()
	if err != nil {
		return nil, err
	}
	result := (*genericattributeprofile.GattServiceProviderResult)(res)
	if code, err := result.GetError(); err != nil {
		return nil, err
	} else if code != winbt.BluetoothErrorSuccess {
		return nil, fmt.Errorf("GATT service provider error %d", code)
	}
	provider, err := result.GetServiceProvider()
	if err != nil {
		return nil, err
	}
	service, err := provider.GetService()
	if err != nil {
		return nil, err
	}
	p := &windowsPeripheral{session: NewSession(hooks), provider: provider}
	writeGUID := winrt.ParameterizedInstanceGUID(
		foundation.GUIDTypedEventHandler,
		genericattributeprofile.SignatureGattLocalCharacteristic,
		genericattributeprofile.SignatureGattWriteRequestedEventArgs,
	)
	p.writeH = foundation.NewTypedEventHandler(ole.NewGUID(writeGUID), func(_ *foundation.TypedEventHandler, sender, args unsafe.Pointer) {
		p.onWrite((*genericattributeprofile.GattWriteRequestedEventArgs)(args))
	})
	command, err := p.addChar(service, CommandUUID, genericattributeprofile.GattCharacteristicPropertiesWriteWithoutResponse|genericattributeprofile.GattCharacteristicPropertiesWrite)
	if err != nil {
		return nil, err
	}
	if _, err := command.AddWriteRequested(p.writeH); err != nil {
		return nil, err
	}
	reply, err := p.addChar(service, ReplyUUID, genericattributeprofile.GattCharacteristicPropertiesNotify)
	if err != nil {
		return nil, err
	}
	p.reply = reply
	subGUID := winrt.ParameterizedInstanceGUID(
		foundation.GUIDTypedEventHandler,
		genericattributeprofile.SignatureGattLocalCharacteristic,
		"cinterface({af86e2e0-b12d-4c6a-9c5a-d7aa65101e90})",
	)
	p.subH = foundation.NewTypedEventHandler(ole.NewGUID(subGUID), func(_ *foundation.TypedEventHandler, _, _ unsafe.Pointer) {
		p.pruneSubscribers()
	})
	if _, err := reply.AddSubscribedClientsChanged(p.subH); err != nil {
		p.subH = nil
	}
	params, err := genericattributeprofile.NewGattServiceProviderAdvertisingParameters()
	if err != nil {
		return nil, err
	}
	if err := params.SetIsConnectable(true); err != nil {
		return nil, err
	}
	if err := params.SetIsDiscoverable(true); err != nil {
		return nil, err
	}
	if err := provider.StartAdvertisingWithParameters(params); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *windowsPeripheral) Note() string {
	return "advertising " + ServiceUUID
}

func (p *windowsPeripheral) Kick() {
	p.apply(p.session.Kick())
}

func (p *windowsPeripheral) Close() error {
	p.Kick()
	if p.provider != nil {
		_ = p.provider.StopAdvertising()
	}
	return nil
}

func (p *windowsPeripheral) addChar(service *genericattributeprofile.GattLocalService, uuid string, props genericattributeprofile.GattCharacteristicProperties) (*genericattributeprofile.GattLocalCharacteristic, error) {
	params, err := genericattributeprofile.NewGattLocalCharacteristicParameters()
	if err != nil {
		return nil, err
	}
	if err := params.SetCharacteristicProperties(props); err != nil {
		return nil, err
	}
	op, err := service.CreateCharacteristicAsync(guid(uuid), params)
	if err != nil {
		return nil, err
	}
	if err := awaitAsync(op, genericattributeprofile.SignatureGattLocalCharacteristicResult); err != nil {
		return nil, err
	}
	res, err := op.GetResults()
	if err != nil {
		return nil, err
	}
	result := (*genericattributeprofile.GattLocalCharacteristicResult)(res)
	return result.GetCharacteristic()
}

func (p *windowsPeripheral) onWrite(args *genericattributeprofile.GattWriteRequestedEventArgs) {
	if args == nil {
		return
	}
	deferral, err := args.GetDeferral()
	if err == nil && deferral != nil {
		defer deferral.Complete()
	}
	client := "windows"
	if session, err := args.GetSession(); err == nil {
		if id, err := gattSessionDeviceID(session); err == nil && id != "" {
			client = id
		}
	}
	op, err := args.GetRequestAsync()
	if err != nil {
		return
	}
	if err := awaitAsync(op, genericattributeprofile.SignatureGattWriteRequest); err != nil {
		return
	}
	res, err := op.GetResults()
	if err != nil {
		return
	}
	req := (*genericattributeprofile.GattWriteRequest)(res)
	buf, err := req.GetValue()
	if err != nil {
		return
	}
	payload := bufferToSlice(buf)
	p.apply(p.session.Handle(client, payload))
	if option, err := req.GetOption(); err == nil && option == genericattributeprofile.GattWriteOptionWriteWithResponse {
		_ = req.Respond()
	}
}

func (p *windowsPeripheral) apply(actions []Action) {
	for _, action := range actions {
		if len(action.Notify) > 0 {
			if action.Broadcast {
				_ = p.notifyAll(action.Notify)
			} else {
				_ = p.notifyAuthed(action.Notify)
			}
		}
		if action.Drop != "" {
			p.session.Drop(action.Drop)
		}
	}
}

func (p *windowsPeripheral) notifyAll(payload []byte) error {
	if p.reply == nil {
		return errors.New("no reply characteristic")
	}
	buf, err := sliceToBuffer(payload)
	if err != nil {
		return err
	}
	op, err := p.reply.NotifyValueAsync(buf)
	if err != nil {
		return err
	}
	signature := fmt.Sprintf("pinterface({%s};%s)", collections.GUIDIVectorView, genericattributeprofile.SignatureGattClientNotificationResult)
	return awaitAsync(op, signature)
}

func (p *windowsPeripheral) notifyAuthed(payload []byte) error {
	authed := p.session.Authed()
	client := p.subscribed(authed)
	if client == nil {
		return p.notifyAll(payload)
	}
	buf, err := sliceToBuffer(payload)
	if err != nil {
		return err
	}
	op, err := p.reply.NotifyValueForSubscribedClientAsync(buf, client)
	if err != nil {
		return err
	}
	return awaitAsync(op, genericattributeprofile.SignatureGattClientNotificationResult)
}

func (p *windowsPeripheral) subscribed(id string) *genericattributeprofile.GattSubscribedClient {
	if p.reply == nil || id == "" {
		return nil
	}
	vec, err := p.reply.GetSubscribedClients()
	if err != nil || vec == nil {
		return nil
	}
	size, err := vec.GetSize()
	if err != nil {
		return nil
	}
	for i := uint32(0); i < size; i++ {
		element, err := vec.GetAt(i)
		if err != nil {
			continue
		}
		client := (*genericattributeprofile.GattSubscribedClient)(element)
		session, err := client.GetSession()
		if err != nil {
			continue
		}
		got, err := gattSessionDeviceID(session)
		if err == nil && got == id {
			return client
		}
	}
	return nil
}

func (p *windowsPeripheral) pruneSubscribers() {
	authed := p.session.Authed()
	if authed == "" {
		return
	}
	if p.subscribed(authed) == nil {
		p.session.Drop(authed)
	}
}

func guid(uuid string) syscall.GUID {
	g := ole.NewGUID(uuid)
	return syscall.GUID{Data1: g.Data1, Data2: g.Data2, Data3: g.Data3, Data4: g.Data4}
}

func awaitAsync(op *foundation.IAsyncOperation, signature string) error {
	if op == nil {
		return errors.New("nil async operation")
	}
	var status foundation.AsyncStatus
	done := make(chan struct{})
	iid := winrt.ParameterizedInstanceGUID(foundation.GUIDAsyncOperationCompletedHandler, signature)
	handler := foundation.NewAsyncOperationCompletedHandler(ole.NewGUID(iid), func(_ *foundation.AsyncOperationCompletedHandler, _ *foundation.IAsyncOperation, asyncStatus foundation.AsyncStatus) {
		status = asyncStatus
		close(done)
	})
	defer handler.Release()
	if err := op.SetCompleted(handler); err != nil {
		return err
	}
	<-done
	if status != foundation.AsyncStatusCompleted {
		return fmt.Errorf("async operation status %d", status)
	}
	return nil
}

func bufferToSlice(buffer *streams.IBuffer) []byte {
	if buffer == nil {
		return nil
	}
	reader, err := streams.DataReaderFromBuffer(buffer)
	if err != nil {
		return nil
	}
	defer reader.Release()
	n, err := buffer.GetLength()
	if err != nil || n == 0 {
		return nil
	}
	data, _ := reader.ReadBytes(n)
	return data
}

func sliceToBuffer(payload []byte) (*streams.IBuffer, error) {
	writer, err := streams.NewDataWriter()
	if err != nil {
		return nil, err
	}
	defer writer.Release()
	if err := writer.WriteBytes(uint32(len(payload)), payload); err != nil {
		return nil, err
	}
	return writer.DetachBuffer()
}

func gattSessionDeviceID(session *genericattributeprofile.GattSession) (string, error) {
	if session == nil {
		return "", errors.New("nil session")
	}
	itf := session.MustQueryInterface(ole.NewGUID("d23b5143-e04e-4c24-999c-9c256f9856b1"))
	defer itf.Release()
	type inspectable struct {
		ole.IInspectable
	}
	type vtbl struct {
		ole.IInspectableVtbl
		GetDeviceId uintptr
	}
	obj := (*inspectable)(unsafe.Pointer(itf))
	vt := (*vtbl)(unsafe.Pointer(obj.RawVTable))
	var out *winbt.BluetoothDeviceId
	hr, _, _ := syscall.SyscallN(vt.GetDeviceId, uintptr(unsafe.Pointer(obj)), uintptr(unsafe.Pointer(&out)))
	if hr != 0 {
		return "", ole.NewError(hr)
	}
	if out == nil {
		return "", errors.New("nil device id")
	}
	return out.GetId()
}
