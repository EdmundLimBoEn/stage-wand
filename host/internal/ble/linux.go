//go:build linux

package ble

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/EdmundLimBoEn/stage-wand/host/internal/protocol"
	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
)

const (
	appPath      = "/systems/edmundlim/stagewand"
	servicePath  = appPath + "/service0"
	commandPath  = servicePath + "/command"
	replyPath    = servicePath + "/reply"
	advPath      = appPath + "/advertisement0"
	gattIface    = "org.bluez.GattCharacteristic1"
	bluezTimeout = 5 * time.Second
)

type linuxPeripheral struct {
	session    *Session
	bus        *dbus.Conn
	adapter    dbus.BusObject
	reply      *prop.Properties
	signals    chan *dbus.Signal
	done       chan struct{}
	watchDone  chan struct{}
	closeOnce  sync.Once
	lifecycle  sync.Mutex
	mu         sync.Mutex
	closed     bool
	running    bool
	bluezOwner string
	lastErr    error
	registered bool
	advertised bool
}

func Start(hooks Hooks, localName string) Peripheral {
	p, err := startLinux(hooks, localName)
	if err != nil {
		return Unavailable(err)
	}
	return p
}

func startLinux(hooks Hooks, localName string) (*linuxPeripheral, error) {
	bus, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}
	p, err := startLinuxBus(hooks, localName, bus)
	if err != nil {
		_ = bus.Close()
	}
	return p, err
}

func startLinuxBus(hooks Hooks, localName string, bus *dbus.Conn) (*linuxPeripheral, error) {
	if localName == "" {
		localName, _ = os.Hostname()
		if localName == "" {
			localName = "StageWand"
		}
	}
	p := &linuxPeripheral{bus: bus, done: make(chan struct{}), watchDone: make(chan struct{}), signals: make(chan *dbus.Signal, 32)}
	hooks.OnActions = p.apply
	p.session = NewSession(hooks)
	if err := p.exportGATT(); err != nil {
		return nil, err
	}
	if err := p.exportAdvertisement(localName); err != nil {
		return nil, err
	}
	p.bus.Signal(p.signals)
	matches := [][]dbus.MatchOption{
		{dbus.WithMatchSender("org.bluez"), dbus.WithMatchInterface("org.freedesktop.DBus.Properties"), dbus.WithMatchMember("PropertiesChanged")},
		{dbus.WithMatchSender("org.bluez"), dbus.WithMatchInterface("org.freedesktop.DBus.ObjectManager"), dbus.WithMatchMember("InterfacesRemoved")},
		{dbus.WithMatchSender("org.freedesktop.DBus"), dbus.WithMatchInterface("org.freedesktop.DBus"), dbus.WithMatchMember("NameOwnerChanged"), dbus.WithMatchArg(0, "org.bluez")},
	}
	for _, match := range matches {
		ctx, cancel := context.WithTimeout(context.Background(), bluezTimeout)
		err := p.bus.AddMatchSignalContext(ctx, match...)
		cancel()
		if err != nil {
			p.bus.RemoveSignal(p.signals)
			return nil, fmt.Errorf("watch Bluetooth state: %w", err)
		}
	}
	p.activate()
	go p.watch()
	return p, nil
}

func (p *linuxPeripheral) Note() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return "closed"
	}
	if !p.bus.Connected() {
		return "unavailable (system D-Bus disconnected; restart Stage Wand)"
	}
	if p.running {
		return "advertising " + ServiceUUID
	}
	if p.lastErr != nil {
		return "unavailable (" + p.lastErr.Error() + "); retrying"
	}
	return "unavailable; retrying"
}

func (p *linuxPeripheral) Kick() { p.session.Kick() }

func (p *linuxPeripheral) Close() error {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closed = true
		p.running = false
		p.mu.Unlock()
		close(p.done)
		p.session.Close()
		p.lifecycle.Lock()
		p.unregister()
		p.bus.RemoveSignal(p.signals)
		_ = p.bus.Close()
		p.lifecycle.Unlock()
		<-p.watchDone
	})
	return nil
}

func bluezCall(object dbus.BusObject, method string, args ...interface{}) *dbus.Call {
	ctx, cancel := context.WithTimeout(context.Background(), bluezTimeout)
	defer cancel()
	return object.CallWithContext(ctx, method, 0, args...)
}

func (p *linuxPeripheral) activate() {
	p.lifecycle.Lock()
	defer p.lifecycle.Unlock()
	p.mu.Lock()
	if p.closed || p.running {
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()
	p.unregister()
	var owner string
	if err := bluezCall(p.bus.BusObject(), "org.freedesktop.DBus.GetNameOwner", "org.bluez").Store(&owner); err != nil {
		p.setError(fmt.Errorf("BlueZ is not running: %w", err))
		return
	}
	p.mu.Lock()
	p.bluezOwner = owner
	p.mu.Unlock()
	adapter, err := findAdapter(p.bus)
	if err != nil {
		p.setError(err)
		return
	}
	p.mu.Lock()
	p.adapter = adapter
	p.mu.Unlock()
	var powered dbus.Variant
	if err := bluezCall(adapter, "org.freedesktop.DBus.Properties.Get", "org.bluez.Adapter1", "Powered").Store(&powered); err != nil {
		p.setError(fmt.Errorf("read adapter power: %w", err))
		return
	}
	if on, _ := powered.Value().(bool); !on {
		p.setError(fmt.Errorf("Bluetooth adapter is powered off"))
		return
	}
	if err := bluezCall(adapter, "org.bluez.GattManager1.RegisterApplication", dbus.ObjectPath(appPath), map[string]dbus.Variant{}).Err; err != nil {
		p.setError(fmt.Errorf("register GATT application: %w", err))
		return
	}
	p.registered = true
	if err := bluezCall(adapter, "org.bluez.LEAdvertisingManager1.RegisterAdvertisement", dbus.ObjectPath(advPath), map[string]dbus.Variant{}).Err; err != nil {
		p.unregister()
		p.setError(fmt.Errorf("register advertisement: %w", err))
		return
	}
	p.advertised = true
	p.mu.Lock()
	p.running, p.lastErr = !p.closed, nil
	p.mu.Unlock()
}

func (p *linuxPeripheral) unregister() {
	if p.advertised {
		_ = bluezCall(p.adapter, "org.bluez.LEAdvertisingManager1.UnregisterAdvertisement", dbus.ObjectPath(advPath)).Err
		p.advertised = false
	}
	if p.registered {
		_ = bluezCall(p.adapter, "org.bluez.GattManager1.UnregisterApplication", dbus.ObjectPath(appPath)).Err
		p.registered = false
	}
}

func (p *linuxPeripheral) setError(err error) {
	p.mu.Lock()
	p.running, p.lastErr = false, err
	p.mu.Unlock()
	if p.reply != nil {
		p.reply.SetMust(gattIface, "Notifying", false)
	}
	p.session.Drop(p.session.Authed())
}

func (p *linuxPeripheral) exportGATT() error {
	objects := map[dbus.ObjectPath]map[string]map[string]dbus.Variant{
		servicePath: {"org.bluez.GattService1": {"UUID": dbus.MakeVariant(strings.ToLower(ServiceUUID)), "Primary": dbus.MakeVariant(true)}},
		commandPath: {gattIface: {"UUID": dbus.MakeVariant(strings.ToLower(CommandUUID)), "Service": dbus.MakeVariant(dbus.ObjectPath(servicePath)), "Flags": dbus.MakeVariant([]string{"write-without-response", "write"})}},
		replyPath:   {gattIface: {"UUID": dbus.MakeVariant(strings.ToLower(ReplyUUID)), "Service": dbus.MakeVariant(dbus.ObjectPath(servicePath)), "Flags": dbus.MakeVariant([]string{"notify"}), "Value": dbus.MakeVariant([]byte{}), "Notifying": dbus.MakeVariant(false)}},
	}
	if err := p.bus.Export(&objectManager{objects: objects}, appPath, "org.freedesktop.DBus.ObjectManager"); err != nil {
		return err
	}
	if _, err := prop.Export(p.bus, servicePath, map[string]map[string]*prop.Prop{
		"org.bluez.GattService1": {"UUID": {Value: strings.ToLower(ServiceUUID)}, "Primary": {Value: true}},
	}); err != nil {
		return err
	}
	if _, err := prop.Export(p.bus, commandPath, map[string]map[string]*prop.Prop{
		gattIface: {"UUID": {Value: strings.ToLower(CommandUUID)}, "Service": {Value: dbus.ObjectPath(servicePath)}, "Flags": {Value: []string{"write-without-response", "write"}}},
	}); err != nil {
		return err
	}
	replyProps, err := prop.Export(p.bus, replyPath, map[string]map[string]*prop.Prop{
		gattIface: {"UUID": {Value: strings.ToLower(ReplyUUID)}, "Service": {Value: dbus.ObjectPath(servicePath)}, "Flags": {Value: []string{"notify"}}, "Value": {Value: []byte{}, Emit: prop.EmitFalse}, "Notifying": {Value: false, Emit: prop.EmitFalse}},
	})
	if err != nil {
		return err
	}
	p.reply = replyProps
	command := &gattChar{role: "command", peripheral: p}
	reply := &gattChar{role: "reply", peripheral: p}
	if err := p.bus.Export(command, commandPath, gattIface); err != nil {
		return err
	}
	return p.bus.Export(reply, replyPath, gattIface)
}

func (p *linuxPeripheral) exportAdvertisement(localName string) error {
	if _, err := prop.Export(p.bus, advPath, map[string]map[string]*prop.Prop{
		"org.bluez.LEAdvertisement1": {"Type": {Value: "peripheral"}, "ServiceUUIDs": {Value: []string{strings.ToLower(ServiceUUID)}}, "LocalName": {Value: localName}, "Timeout": {Value: uint16(0)}},
	}); err != nil {
		return err
	}
	return p.bus.Export(&advertisement{peripheral: p}, advPath, "org.bluez.LEAdvertisement1")
}

func (p *linuxPeripheral) apply(actions []Action) {
	for _, action := range actions {
		if len(action.Notify) > 0 && p.reply != nil {
			// BlueZ Value notifications reach every subscriber. Never broadcast a
			// rejected second central's auth failure to the active controller.
			owner := p.session.Authed()
			if (owner == "" || owner == action.Client) && p.reply.GetMust(gattIface, "Notifying").(bool) {
				payload := append([]byte(nil), action.Notify...)
				p.reply.SetMust(gattIface, "Value", payload)
				_ = p.bus.Emit(replyPath, "org.freedesktop.DBus.Properties.PropertiesChanged", gattIface, map[string]dbus.Variant{"Value": dbus.MakeVariant(payload)}, []string{})
			}
		}
		if action.Drop != "" {
			_ = bluezCall(p.bus.Object("org.bluez", dbus.ObjectPath(action.Drop)), "org.bluez.Device1.Disconnect").Err
		}
	}
}

func (p *linuxPeripheral) watch() {
	defer close(p.watchDone)
	retry := time.NewTicker(5 * time.Second)
	defer retry.Stop()
	for {
		select {
		case <-p.done:
			return
		case <-p.bus.Context().Done():
			p.setError(fmt.Errorf("system D-Bus disconnected; restart Stage Wand"))
			return
		case <-retry.C:
			p.activate()
		case sig, ok := <-p.signals:
			if !ok {
				p.setError(fmt.Errorf("system D-Bus disconnected; restart Stage Wand"))
				return
			}
			p.handleSignal(sig)
		}
	}
}

func (p *linuxPeripheral) handleSignal(sig *dbus.Signal) {
	if sig == nil {
		return
	}
	if sig.Sender == "org.freedesktop.DBus" && sig.Name == "org.freedesktop.DBus.NameOwnerChanged" && len(sig.Body) == 3 {
		name, _ := sig.Body[0].(string)
		if name == "org.bluez" {
			p.mu.Lock()
			p.bluezOwner = ""
			p.mu.Unlock()
			p.setError(fmt.Errorf("BlueZ restarted"))
		}
		return
	}
	p.mu.Lock()
	trusted := p.bluezOwner != "" && sig.Sender == p.bluezOwner
	adapter := p.adapter
	p.mu.Unlock()
	if !trusted {
		return
	}
	if sig.Name == "org.freedesktop.DBus.ObjectManager.InterfacesRemoved" && len(sig.Body) == 2 {
		path, _ := sig.Body[0].(dbus.ObjectPath)
		interfaces, _ := sig.Body[1].([]string)
		for _, iface := range interfaces {
			if iface == "org.bluez.Device1" {
				p.session.Drop(string(path))
			}
			if iface == "org.bluez.Adapter1" && adapter != nil && path == adapter.Path() {
				p.setError(fmt.Errorf("Bluetooth adapter removed"))
			}
		}
	}
	if sig.Name != "org.freedesktop.DBus.Properties.PropertiesChanged" || len(sig.Body) < 2 {
		return
	}
	iface, _ := sig.Body[0].(string)
	changes, _ := sig.Body[1].(map[string]dbus.Variant)
	if iface == "org.bluez.Device1" {
		if value, ok := changes["Connected"]; ok {
			if connected, valid := value.Value().(bool); valid && !connected {
				p.session.Drop(string(sig.Path))
			}
		}
	}
	if iface == "org.bluez.Adapter1" && adapter != nil && sig.Path == adapter.Path() {
		if value, ok := changes["Powered"]; ok {
			if powered, valid := value.Value().(bool); valid && !powered {
				p.setError(fmt.Errorf("Bluetooth adapter is powered off"))
			}
		}
	}
}

func findAdapter(bus *dbus.Conn) (dbus.BusObject, error) {
	var objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	if err := bluezCall(bus.Object("org.bluez", "/"), "org.freedesktop.DBus.ObjectManager.GetManagedObjects").Store(&objects); err != nil {
		return nil, fmt.Errorf("BlueZ: %w", err)
	}
	var paths []string
	for path, ifaces := range objects {
		_, adapter := ifaces["org.bluez.Adapter1"]
		_, gatt := ifaces["org.bluez.GattManager1"]
		_, advertising := ifaces["org.bluez.LEAdvertisingManager1"]
		if adapter && gatt && advertising {
			paths = append(paths, string(path))
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no Bluetooth adapter supports GATT hosting and LE advertising")
	}
	sort.Slice(paths, func(i, j int) bool {
		left, _ := objects[dbus.ObjectPath(paths[i])]["org.bluez.Adapter1"]["Powered"].Value().(bool)
		right, _ := objects[dbus.ObjectPath(paths[j])]["org.bluez.Adapter1"]["Powered"].Value().(bool)
		if left != right {
			return left
		}
		return paths[i] < paths[j]
	})
	return bus.Object("org.bluez", dbus.ObjectPath(paths[0])), nil
}

type objectManager struct {
	objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant
}

func (om *objectManager) GetManagedObjects() (map[dbus.ObjectPath]map[string]map[string]dbus.Variant, *dbus.Error) {
	return om.objects, nil
}

type advertisement struct{ peripheral *linuxPeripheral }

func (a *advertisement) Release(sender dbus.Sender) *dbus.Error {
	if !a.peripheral.trusted(sender) {
		return bluezError("NotAuthorized", "Only BlueZ may release advertisements")
	}
	a.peripheral.setError(fmt.Errorf("Bluetooth advertisement was released"))
	return nil
}

func (p *linuxPeripheral) trusted(sender dbus.Sender) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.closed && p.bluezOwner != "" && string(sender) == p.bluezOwner
}

type gattChar struct {
	role       string
	peripheral *linuxPeripheral
}

func (c *gattChar) ReadValue(options map[string]dbus.Variant) ([]byte, *dbus.Error) {
	return nil, bluezError("NotPermitted", "Characteristic does not support reads")
}

func (c *gattChar) WriteValue(sender dbus.Sender, value []byte, options map[string]dbus.Variant) *dbus.Error {
	p := c.peripheral
	if !p.trusted(sender) {
		return bluezError("NotAuthorized", "Only BlueZ may write commands")
	}
	if c.role != "command" {
		return bluezError("NotPermitted", "Characteristic does not support writes")
	}
	p.mu.Lock()
	running := p.running
	p.mu.Unlock()
	if !running {
		return bluezError("NotPermitted", "Bluetooth service is unavailable")
	}
	if err := validateWrite(value, options); err != nil {
		return err
	}
	client := clientFromOptions(options)
	if client == "" {
		return bluezError("NotAuthorized", "Missing Bluetooth device identity")
	}
	if !p.reply.GetMust(gattIface, "Notifying").(bool) {
		return bluezError("NotPermitted", "Subscribe to replies before sending commands")
	}
	p.session.Handle(client, value)
	p.mu.Lock()
	running = p.running
	p.mu.Unlock()
	if !running || !p.reply.GetMust(gattIface, "Notifying").(bool) {
		p.session.Drop(client)
	}
	return nil
}

func validateWrite(value []byte, options map[string]dbus.Variant) *dbus.Error {
	if len(value) == 0 || len(value) > protocol.MaxFrameBytes {
		return bluezError("InvalidValueLength", "Expected one complete JSON command")
	}
	if offset, ok := options["offset"]; ok {
		if n, valid := offset.Value().(uint16); !valid || n != 0 {
			return bluezError("InvalidOffset", "Partial writes are not supported")
		}
	}
	if prepared, ok := options["prepare-authorize"]; ok {
		if enabled, valid := prepared.Value().(bool); !valid || enabled {
			return bluezError("NotSupported", "Prepared writes are not supported")
		}
	}
	if kind, ok := options["type"]; ok {
		if value := kind.Value(); value != "command" && value != "request" {
			return bluezError("NotSupported", "Expected a single ATT write")
		}
	}
	if mtu, ok := options["mtu"]; ok {
		if n, valid := mtu.Value().(uint16); !valid || int(n)-3 < protocol.MinimumCommandBytes || len(value) > int(n)-3 {
			return bluezError("InvalidValueLength", "Negotiate an MTU large enough for the command")
		}
	}
	return nil
}

func (c *gattChar) StartNotify(sender dbus.Sender) *dbus.Error {
	if !c.peripheral.trusted(sender) {
		return bluezError("NotAuthorized", "Only BlueZ may subscribe")
	}
	if c.role != "reply" {
		return bluezError("NotSupported", "Characteristic does not support notifications")
	}
	c.peripheral.reply.SetMust(gattIface, "Notifying", true)
	return nil
}

func (c *gattChar) StopNotify(sender dbus.Sender) *dbus.Error {
	if !c.peripheral.trusted(sender) {
		return bluezError("NotAuthorized", "Only BlueZ may unsubscribe")
	}
	if c.role != "reply" {
		return bluezError("NotSupported", "Characteristic does not support notifications")
	}
	c.peripheral.reply.SetMust(gattIface, "Notifying", false)
	c.peripheral.session.Drop(c.peripheral.session.Authed())
	return nil
}

func bluezError(name, message string) *dbus.Error {
	return dbus.NewError("org.bluez.Error."+name, []interface{}{message})
}

func clientFromOptions(options map[string]dbus.Variant) string {
	path, ok := options["device"].Value().(dbus.ObjectPath)
	if !ok || !path.IsValid() {
		return ""
	}
	parts := strings.Split(string(path), "/")
	if len(parts) != 5 || parts[1] != "org" || parts[2] != "bluez" || !strings.HasPrefix(parts[3], "hci") || !strings.HasPrefix(parts[4], "dev_") {
		return ""
	}
	if len(parts[3]) == 3 {
		return ""
	}
	for _, digit := range parts[3][3:] {
		if digit < '0' || digit > '9' {
			return ""
		}
	}
	address := strings.Split(strings.TrimPrefix(parts[4], "dev_"), "_")
	if len(address) != 6 {
		return ""
	}
	for _, octet := range address {
		if len(octet) != 2 {
			return ""
		}
		for _, digit := range octet {
			if !strings.ContainsRune("0123456789ABCDEFabcdef", digit) {
				return ""
			}
		}
	}
	return string(path)
}
