//go:build linux

package ble

import (
	"bufio"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/EdmundLimBoEn/stage-wand/host/internal/control"
	"github.com/EdmundLimBoEn/stage-wand/host/internal/protocol"
	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
)

const testAdapterPath dbus.ObjectPath = "/org/bluez/hci0"
const testDevicePath dbus.ObjectPath = "/org/bluez/hci0/dev_00_11_22_33_44_55"

func TestBlueZWriteValidation(t *testing.T) {
	valid := []byte(`{"t":"auth","code":"0000"}`)
	for _, tc := range []struct {
		name    string
		payload []byte
		options map[string]dbus.Variant
		want    string
	}{
		{"normal", valid, nil, ""},
		{"negotiated MTU", valid, map[string]dbus.Variant{"mtu": dbus.MakeVariant(uint16(185))}, ""},
		{"empty", nil, nil, "InvalidValueLength"},
		{"oversize", make([]byte, 513), nil, "InvalidValueLength"},
		{"offset", valid, map[string]dbus.Variant{"offset": dbus.MakeVariant(uint16(1))}, "InvalidOffset"},
		{"wrong offset type", valid, map[string]dbus.Variant{"offset": dbus.MakeVariant("0")}, "InvalidOffset"},
		{"prepared", valid, map[string]dbus.Variant{"prepare-authorize": dbus.MakeVariant(true)}, "NotSupported"},
		{"reliable", valid, map[string]dbus.Variant{"type": dbus.MakeVariant("reliable")}, "NotSupported"},
		{"default MTU", valid, map[string]dbus.Variant{"mtu": dbus.MakeVariant(uint16(23))}, "InvalidValueLength"},
		{"MTU fits auth but not movement", valid, map[string]dbus.Variant{"mtu": dbus.MakeVariant(uint16(40))}, "InvalidValueLength"},
		{"minimum MTU", valid, map[string]dbus.Variant{"mtu": dbus.MakeVariant(uint16(protocol.MinimumCommandBytes + 3))}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateWrite(tc.payload, tc.options)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || err.Name != "org.bluez.Error."+tc.want {
				t.Fatalf("got %v, want %s", err, tc.want)
			}
		})
	}
}

func TestBlueZDeviceIdentityCannotNameArbitraryDBusObjects(t *testing.T) {
	for _, value := range []interface{}{nil, string(testDevicePath), 7, dbus.ObjectPath("/org/bluez/hci0"), dbus.ObjectPath("/elsewhere/hci0/dev_00_11_22_33_44_55"), dbus.ObjectPath("/org/bluez/hci0/dev_ZZ_11_22_33_44_55"), dbus.ObjectPath("/org/bluez/hci/dev_00_11_22_33_44_55")} {
		options := map[string]dbus.Variant{}
		if value != nil {
			options["device"] = dbus.MakeVariant(value)
		}
		if got := clientFromOptions(options); got != "" {
			t.Fatalf("accepted %v as %q", value, got)
		}
	}
	if got := clientFromOptions(map[string]dbus.Variant{"device": dbus.MakeVariant(testDevicePath)}); got != string(testDevicePath) {
		t.Fatal(got)
	}
}

type fakeAdapter struct {
	mu                                                              sync.Mutex
	applications, advertisements, unregisteredApps, unregisteredAds int
	failAdvertising                                                 bool
}

func (f *fakeAdapter) RegisterApplication(_ dbus.ObjectPath, _ map[string]dbus.Variant) *dbus.Error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applications++
	return nil
}
func (f *fakeAdapter) UnregisterApplication(_ dbus.ObjectPath) *dbus.Error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unregisteredApps++
	return nil
}
func (f *fakeAdapter) RegisterAdvertisement(_ dbus.ObjectPath, _ map[string]dbus.Variant) *dbus.Error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.advertisements++
	if f.failAdvertising {
		return bluezError("Failed", "adapter busy")
	}
	return nil
}
func (f *fakeAdapter) UnregisterAdvertisement(_ dbus.ObjectPath) *dbus.Error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unregisteredAds++
	return nil
}

type fakeDevice struct {
	mu          sync.Mutex
	disconnects int
}

func (d *fakeDevice) Disconnect() *dbus.Error {
	d.mu.Lock()
	d.disconnects++
	d.mu.Unlock()
	return nil
}

type fakeObjectManager struct {
	mu      sync.Mutex
	objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant
}

func (m *fakeObjectManager) GetManagedObjects() (map[dbus.ObjectPath]map[string]map[string]dbus.Variant, *dbus.Error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.objects, nil
}

type testBlueZ struct {
	bus, client *dbus.Conn
	adapter     *fakeAdapter
	properties  *prop.Properties
	objects     *fakeObjectManager
	device      *fakeDevice
	address     string
}

func isolatedBlueZ(t *testing.T) *testBlueZ {
	t.Helper()
	binary, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("dbus-daemon is required for BlueZ integration tests")
	}
	command := exec.Command(binary, "--session", "--nofork", "--nopidfile", "--print-address=1")
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait() })
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() {
		t.Fatal("no D-Bus address")
	}
	address := scanner.Text()
	bus, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bus.Close() })
	client, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if _, err := bus.RequestName("org.bluez", dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}
	mock := &testBlueZ{bus: bus, client: client, adapter: &fakeAdapter{}, device: &fakeDevice{}, address: address}
	mock.objects = &fakeObjectManager{objects: map[dbus.ObjectPath]map[string]map[string]dbus.Variant{
		testAdapterPath: {"org.bluez.Adapter1": {"Powered": dbus.MakeVariant(true)}, "org.bluez.GattManager1": {}, "org.bluez.LEAdvertisingManager1": {}},
	}}
	for _, item := range []struct {
		object interface{}
		path   dbus.ObjectPath
		iface  string
	}{
		{mock.objects, "/", "org.freedesktop.DBus.ObjectManager"},
		{mock.adapter, testAdapterPath, "org.bluez.GattManager1"},
		{mock.adapter, testAdapterPath, "org.bluez.LEAdvertisingManager1"},
		{mock.device, testDevicePath, "org.bluez.Device1"},
	} {
		if err := bus.Export(item.object, item.path, item.iface); err != nil {
			t.Fatal(err)
		}
	}
	mock.properties, err = prop.Export(bus, testAdapterPath, map[string]map[string]*prop.Prop{"org.bluez.Adapter1": {"Powered": {Value: true, Writable: true, Emit: prop.EmitTrue}}})
	if err != nil {
		t.Fatal(err)
	}
	return mock
}

func (b *testBlueZ) start(t *testing.T, hooks Hooks) *linuxPeripheral {
	t.Helper()
	if hooks.Control == nil {
		hooks.Control = control.NewSession("0000")
	}
	p, err := startLinuxBus(hooks, "StageWand-test", b.client)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func (b *testBlueZ) object(p *linuxPeripheral, path dbus.ObjectPath) dbus.BusObject {
	return b.bus.Object(p.bus.Names()[0], path)
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(7 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition did not become true")
}

func TestBlueZAuthenticationLifecycleAndLocalSpoofing(t *testing.T) {
	b := isolatedBlueZ(t)
	controller := control.NewSession("0000")
	var mu sync.Mutex
	commands := 0
	p := b.start(t, Hooks{Control: controller, OnCommand: func(protocol.Command) { mu.Lock(); commands++; mu.Unlock() }})
	if !strings.HasPrefix(p.Note(), "advertising ") {
		t.Fatal(p.Note())
	}
	command := b.object(p, commandPath)
	reply := b.object(p, replyPath)
	options := map[string]dbus.Variant{"device": dbus.MakeVariant(testDevicePath), "mtu": dbus.MakeVariant(uint16(185))}
	auth := []byte(`{"t":"auth","code":"0000"}`)
	if err := command.Call(gattIface+".WriteValue", 0, auth, options).Err; err == nil {
		t.Fatal("auth accepted before subscription")
	}
	if err := reply.Call(gattIface+".StartNotify", 0).Err; err != nil {
		t.Fatal(err)
	}
	if err := command.Call(gattIface+".WriteValue", 0, auth, options).Err; err != nil {
		t.Fatal(err)
	}
	if p.session.Authed() != string(testDevicePath) {
		t.Fatal("not authenticated")
	}
	if err := command.Call(gattIface+".WriteValue", 0, []byte(`{"t":"key","k":"right"}`), options).Err; err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	got := commands
	mu.Unlock()
	if got != 1 {
		t.Fatalf("commands %d", got)
	}
	intruder, err := dbus.Connect(b.address)
	if err != nil {
		t.Fatal(err)
	}
	defer intruder.Close()
	for _, call := range []struct {
		path   dbus.ObjectPath
		method string
		args   []interface{}
	}{
		{commandPath, gattIface + ".WriteValue", []interface{}{[]byte(`{"t":"key","k":"right"}`), options}},
		{replyPath, gattIface + ".StopNotify", nil},
		{replyPath, "org.freedesktop.DBus.Properties.Set", []interface{}{gattIface, "Value", dbus.MakeVariant([]byte(`{"t":"status"}`))}},
	} {
		if err := intruder.Object(p.bus.Names()[0], call.path).Call(call.method, 0, call.args...).Err; err == nil {
			t.Fatalf("local untrusted process may invoke %s", call.method)
		}
	}
	if p.session.Authed() != string(testDevicePath) {
		t.Fatal("spoofed unsubscribe revoked owner")
	}
	if err := reply.Call(gattIface+".StopNotify", 0).Err; err != nil {
		t.Fatal(err)
	}
	if controller.Peer() != "" {
		t.Fatal("unsubscribe retained owner")
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if p.Note() != "closed" {
		t.Fatal(p.Note())
	}
	b.adapter.mu.Lock()
	apps, ads := b.adapter.unregisteredApps, b.adapter.unregisteredAds
	b.adapter.mu.Unlock()
	if apps != 1 || ads != 1 {
		t.Fatalf("cleanup: applications=%d advertisements=%d", apps, ads)
	}
}

func TestBlueZRejectsSecondCentralWithoutBroadcastingBye(t *testing.T) {
	b := isolatedBlueZ(t)
	p := b.start(t, Hooks{})
	if err := b.object(p, replyPath).Call(gattIface+".StartNotify", 0).Err; err != nil {
		t.Fatal(err)
	}
	p.session.Handle(string(testDevicePath), []byte(`{"t":"auth","code":"0000"}`))
	other := "/org/bluez/hci0/dev_AA_BB_CC_DD_EE_FF"
	device := &fakeDevice{}
	if err := b.bus.Export(device, dbus.ObjectPath(other), "org.bluez.Device1"); err != nil {
		t.Fatal(err)
	}
	p.session.Handle(other, []byte(`{"t":"auth","code":"9999"}`))
	if got := string(p.reply.GetMust(gattIface, "Value").([]byte)); got != `{"t":"status"}` {
		t.Fatalf("broadcast other client's rejection: %s", got)
	}
	device.mu.Lock()
	disconnected := device.disconnects
	device.mu.Unlock()
	if disconnected != 1 || p.session.Authed() != string(testDevicePath) {
		t.Fatal("second central rejection disturbed owner or kept rejected device")
	}
}

func TestBlueZDisconnectAndDaemonRestart(t *testing.T) {
	b := isolatedBlueZ(t)
	p := b.start(t, Hooks{})
	p.session.Handle(string(testDevicePath), []byte(`{"t":"auth","code":"0000"}`))
	if err := b.bus.Emit(testDevicePath, "org.freedesktop.DBus.Properties.PropertiesChanged", "org.bluez.Device1", map[string]dbus.Variant{"Connected": dbus.MakeVariant(false)}, []string{}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return p.session.Authed() == "" })
	p.session.Handle(string(testDevicePath), []byte(`{"t":"auth","code":"0000"}`))
	if _, err := b.bus.ReleaseName("org.bluez"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return strings.Contains(p.Note(), "unavailable") && p.session.Authed() == "" })
	if _, err := b.bus.RequestName("org.bluez", dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return strings.HasPrefix(p.Note(), "advertising ") })
	b.adapter.mu.Lock()
	applications := b.adapter.applications
	b.adapter.mu.Unlock()
	if applications != 2 {
		t.Fatalf("registered %d applications, want restart registration", applications)
	}
	if p.reply.GetMust(gattIface, "Notifying").(bool) {
		t.Fatal("restart retained stale subscription")
	}
}

func TestBlueZAdvertisingFailureRollsBackAndRetries(t *testing.T) {
	b := isolatedBlueZ(t)
	b.adapter.mu.Lock()
	b.adapter.failAdvertising = true
	b.adapter.mu.Unlock()
	p := b.start(t, Hooks{})
	if !strings.Contains(p.Note(), "adapter busy") {
		t.Fatal(p.Note())
	}
	b.adapter.mu.Lock()
	unregistered := b.adapter.unregisteredApps
	b.adapter.failAdvertising = false
	b.adapter.mu.Unlock()
	if unregistered != 1 {
		t.Fatal("failed advertisement left GATT registered")
	}
	p.activate()
	if !strings.HasPrefix(p.Note(), "advertising ") {
		t.Fatal(p.Note())
	}
}

func TestBlueZBusLossClearsOwnerAndStopsWatcher(t *testing.T) {
	b := isolatedBlueZ(t)
	p := b.start(t, Hooks{})
	p.session.Handle(string(testDevicePath), []byte(`{"t":"auth","code":"0000"}`))
	if err := p.bus.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.watchDone:
	case <-time.After(time.Second):
		t.Fatal("watcher survived disconnected D-Bus")
	}
	if p.session.Authed() != "" || !strings.Contains(p.Note(), "restart Stage Wand") || strings.Contains(p.Note(), "retrying") {
		t.Fatalf("stale bus loss status: owner=%q note=%s", p.session.Authed(), p.Note())
	}
}

func TestBlueZPoweredOffAdapterIsNotEnabledByApp(t *testing.T) {
	b := isolatedBlueZ(t)
	b.properties.SetMust("org.bluez.Adapter1", "Powered", false)
	p := b.start(t, Hooks{})
	if !strings.Contains(p.Note(), "powered off") {
		t.Fatal(p.Note())
	}
	if b.properties.GetMust("org.bluez.Adapter1", "Powered").(bool) {
		t.Fatal("app enabled disabled radio")
	}
	b.properties.SetMust("org.bluez.Adapter1", "Powered", true)
	p.activate()
	if !strings.HasPrefix(p.Note(), "advertising ") {
		t.Fatal(p.Note())
	}
}

func TestBlueZAdapterMustSupportPeripheralRole(t *testing.T) {
	b := isolatedBlueZ(t)
	b.objects.mu.Lock()
	b.objects.objects = map[dbus.ObjectPath]map[string]map[string]dbus.Variant{
		"/org/bluez/hci0": {"org.bluez.Adapter1": {}},
		"/org/bluez/hci1": {"org.bluez.Adapter1": {"Powered": dbus.MakeVariant(false)}, "org.bluez.GattManager1": {}, "org.bluez.LEAdvertisingManager1": {}},
		"/org/bluez/hci2": {"org.bluez.Adapter1": {"Powered": dbus.MakeVariant(true)}, "org.bluez.GattManager1": {}, "org.bluez.LEAdvertisingManager1": {}},
	}
	b.objects.mu.Unlock()
	adapter, err := findAdapter(b.client)
	if err != nil {
		t.Fatal(err)
	}
	if string(adapter.Path()) != "/org/bluez/hci2" {
		t.Fatalf("selected %s", adapter.Path())
	}
}
