//go:build linux

package ble

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
)

const (
	appPath     = "/systems/edmundlim/stagewand"
	servicePath = appPath + "/service0"
	commandPath = servicePath + "/command"
	replyPath   = servicePath + "/reply"
	advPath     = appPath + "/advertisement0"
)

type linuxPeripheral struct {
	session *Session
	bus     *dbus.Conn
	adapter dbus.BusObject
	reply   *prop.Properties
	mu      sync.Mutex
	closed  bool
}

func Start(hooks Hooks, localName string) Peripheral {
	p, err := startLinux(hooks, localName)
	if err != nil {
		return Unavailable(err)
	}
	return p
}

func startLinux(hooks Hooks, localName string) (*linuxPeripheral, error) {
	bus, err := dbus.SystemBus()
	if err != nil {
		return nil, err
	}
	adapter, err := findAdapter(bus)
	if err != nil {
		return nil, err
	}
	if err := adapter.SetProperty("org.bluez.Adapter1.Powered", true); err != nil {
		powered, getErr := adapter.GetProperty("org.bluez.Adapter1.Powered")
		if getErr != nil {
			return nil, fmt.Errorf("power adapter: %w", err)
		}
		if on, _ := powered.Value().(bool); !on {
			return nil, fmt.Errorf("adapter powered off: %w", err)
		}
	}
	if localName == "" {
		localName, _ = os.Hostname()
		if localName == "" {
			localName = "StageWand"
		}
	}
	p := &linuxPeripheral{
		session: NewSession(hooks),
		bus:     bus,
		adapter: adapter,
	}
	if err := p.exportGATT(); err != nil {
		return nil, err
	}
	if err := p.exportAdvertisement(localName); err != nil {
		return nil, err
	}
	if err := adapter.Call("org.bluez.GattManager1.RegisterApplication", 0, dbus.ObjectPath(appPath), map[string]dbus.Variant(nil)).Err; err != nil {
		return nil, fmt.Errorf("register GATT application: %w", err)
	}
	if err := adapter.Call("org.bluez.LEAdvertisingManager1.RegisterAdvertisement", 0, dbus.ObjectPath(advPath), map[string]interface{}{}).Err; err != nil {
		_ = adapter.Call("org.bluez.GattManager1.UnregisterApplication", 0, dbus.ObjectPath(appPath)).Err
		return nil, fmt.Errorf("register advertisement: %w", err)
	}
	go p.watchDisconnects()
	return p, nil
}

func (p *linuxPeripheral) Note() string {
	return "advertising " + ServiceUUID
}

func (p *linuxPeripheral) Kick() {
	p.apply(p.session.Kick())
}

func (p *linuxPeripheral) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()
	p.Kick()
	_ = p.adapter.Call("org.bluez.LEAdvertisingManager1.UnregisterAdvertisement", 0, dbus.ObjectPath(advPath)).Err
	_ = p.adapter.Call("org.bluez.GattManager1.UnregisterApplication", 0, dbus.ObjectPath(appPath)).Err
	return nil
}

func (p *linuxPeripheral) exportGATT() error {
	objects := map[dbus.ObjectPath]map[string]map[string]dbus.Variant{
		servicePath: {
			"org.bluez.GattService1": {
				"UUID":    dbus.MakeVariant(strings.ToLower(ServiceUUID)),
				"Primary": dbus.MakeVariant(true),
			},
		},
		commandPath: {
			"org.bluez.GattCharacteristic1": {
				"UUID":    dbus.MakeVariant(strings.ToLower(CommandUUID)),
				"Service": dbus.MakeVariant(dbus.ObjectPath(servicePath)),
				"Flags":   dbus.MakeVariant([]string{"write-without-response", "write"}),
			},
		},
		replyPath: {
			"org.bluez.GattCharacteristic1": {
				"UUID":    dbus.MakeVariant(strings.ToLower(ReplyUUID)),
				"Service": dbus.MakeVariant(dbus.ObjectPath(servicePath)),
				"Flags":   dbus.MakeVariant([]string{"notify"}),
				"Value":   dbus.MakeVariant([]byte{}),
			},
		},
	}
	if err := p.bus.Export(&objectManager{objects: objects}, appPath, "org.freedesktop.DBus.ObjectManager"); err != nil {
		return err
	}
	if _, err := prop.Export(p.bus, servicePath, map[string]map[string]*prop.Prop{
		"org.bluez.GattService1": {
			"UUID":    {Value: strings.ToLower(ServiceUUID)},
			"Primary": {Value: true},
		},
	}); err != nil {
		return err
	}
	commandProps, err := prop.Export(p.bus, commandPath, map[string]map[string]*prop.Prop{
		"org.bluez.GattCharacteristic1": {
			"UUID":    {Value: strings.ToLower(CommandUUID)},
			"Service": {Value: dbus.ObjectPath(servicePath)},
			"Flags":   {Value: []string{"write-without-response", "write"}},
			"Value":   {Value: []byte{}, Writable: true},
		},
	})
	if err != nil {
		return err
	}
	replyProps, err := prop.Export(p.bus, replyPath, map[string]map[string]*prop.Prop{
		"org.bluez.GattCharacteristic1": {
			"UUID":    {Value: strings.ToLower(ReplyUUID)},
			"Service": {Value: dbus.ObjectPath(servicePath)},
			"Flags":   {Value: []string{"notify"}},
			"Value":   {Value: []byte{}, Writable: true, Emit: prop.EmitTrue},
		},
	})
	if err != nil {
		return err
	}
	p.reply = replyProps
	command := &gattChar{role: "command", session: p.session, props: commandProps, apply: p.apply}
	reply := &gattChar{role: "reply", props: replyProps}
	if err := p.bus.Export(command, commandPath, "org.bluez.GattCharacteristic1"); err != nil {
		return err
	}
	return p.bus.Export(reply, replyPath, "org.bluez.GattCharacteristic1")
}

func (p *linuxPeripheral) exportAdvertisement(localName string) error {
	if _, err := prop.Export(p.bus, advPath, map[string]map[string]*prop.Prop{
		"org.bluez.LEAdvertisement1": {
			"Type":         {Value: "peripheral"},
			"ServiceUUIDs": {Value: []string{strings.ToLower(ServiceUUID)}},
			"LocalName":    {Value: localName},
			"Timeout":      {Value: uint16(0)},
		},
	}); err != nil {
		return err
	}
	return p.bus.Export(&advertisement{}, advPath, "org.bluez.LEAdvertisement1")
}

func (p *linuxPeripheral) apply(actions []Action) {
	for _, action := range actions {
		if len(action.Notify) > 0 && p.reply != nil {
			_ = p.reply.Set("org.bluez.GattCharacteristic1", "Value", dbus.MakeVariant(action.Notify))
		}
		if action.Drop != "" {
			p.session.Drop(action.Drop)
			_ = p.bus.Object("org.bluez", dbus.ObjectPath(action.Drop)).Call("org.bluez.Device1.Disconnect", 0)
		}
	}
}

func (p *linuxPeripheral) watchDisconnects() {
	signals := make(chan *dbus.Signal, 16)
	p.bus.Signal(signals)
	_ = p.bus.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.DBus.Properties"), dbus.WithMatchMember("PropertiesChanged"))
	for sig := range signals {
		p.mu.Lock()
		closed := p.closed
		p.mu.Unlock()
		if closed {
			return
		}
		if sig.Name != "org.freedesktop.DBus.Properties.PropertiesChanged" || len(sig.Body) < 2 {
			continue
		}
		iface, _ := sig.Body[0].(string)
		if iface != "org.bluez.Device1" {
			continue
		}
		changes, _ := sig.Body[1].(map[string]dbus.Variant)
		connected, ok := changes["Connected"]
		if !ok {
			continue
		}
		on, _ := connected.Value().(bool)
		if on {
			continue
		}
		p.session.Drop(string(sig.Path))
	}
}

func findAdapter(bus *dbus.Conn) (dbus.BusObject, error) {
	var objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	if err := bus.Object("org.bluez", "/").Call("org.freedesktop.DBus.ObjectManager.GetManagedObjects", 0).Store(&objects); err != nil {
		return nil, fmt.Errorf("bluez: %w", err)
	}
	var first dbus.ObjectPath
	for path, ifaces := range objects {
		if _, ok := ifaces["org.bluez.Adapter1"]; !ok {
			continue
		}
		if strings.HasSuffix(string(path), "/hci0") {
			return bus.Object("org.bluez", path), nil
		}
		if first == "" {
			first = path
		}
	}
	if first == "" {
		return nil, fmt.Errorf("no Bluetooth adapter")
	}
	return bus.Object("org.bluez", first), nil
}

type objectManager struct {
	objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant
}

func (om *objectManager) GetManagedObjects() (map[dbus.ObjectPath]map[string]map[string]dbus.Variant, *dbus.Error) {
	return om.objects, nil
}

type advertisement struct{}

func (advertisement) Release() *dbus.Error { return nil }

type gattChar struct {
	role    string
	session *Session
	props   *prop.Properties
	apply   func([]Action)
}

func (c *gattChar) ReadValue(options map[string]dbus.Variant) ([]byte, *dbus.Error) {
	if c.props == nil {
		return []byte{}, nil
	}
	value := c.props.GetMust("org.bluez.GattCharacteristic1", "Value")
	data, _ := value.([]byte)
	return data, nil
}

func (c *gattChar) WriteValue(value []byte, options map[string]dbus.Variant) *dbus.Error {
	if c.role != "command" || c.session == nil {
		return dbus.MakeFailedError(fmt.Errorf("not writable"))
	}
	c.apply(c.session.Handle(clientFromOptions(options), value))
	return nil
}

func (c *gattChar) StartNotify() *dbus.Error { return nil }

func (c *gattChar) StopNotify() *dbus.Error { return nil }

func clientFromOptions(options map[string]dbus.Variant) string {
	v, ok := options["device"]
	if !ok {
		return "unknown"
	}
	switch x := v.Value().(type) {
	case dbus.ObjectPath:
		return string(x)
	case string:
		return x
	default:
		return fmt.Sprint(x)
	}
}
