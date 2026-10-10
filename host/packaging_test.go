package packaging_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchUinputPackaging(t *testing.T) {
	hostDir := testHostDir(t)
	udevPath := filepath.Join(hostDir, "udev", "70-stagewand-uinput.rules")
	udev := readFile(t, udevPath)
	if strings.Contains(udev, `GROUP="input"`) {
		t.Fatal("udev rule must not use GROUP=input; Arch and systemd 258 treat that category group as broken")
	}
	if !strings.Contains(udev, `TAG+="uaccess"`) {
		t.Fatal("udev rule must set TAG+=uaccess")
	}
	if !strings.Contains(udev, `SUBSYSTEM=="misc"`) {
		t.Fatal("udev rule must match SUBSYSTEM misc")
	}
	if filepath.Base(udevPath) != "70-stagewand-uinput.rules" {
		t.Fatal("udev rule must be named 70- so it sorts before 73-seat-late.rules")
	}
	if _, err := os.Stat(filepath.Join(hostDir, "udev", "99-stagewand-uinput.rules")); err == nil {
		t.Fatal("99-stagewand-uinput.rules is too late for uaccess")
	}
	modules := strings.TrimSpace(readFile(t, filepath.Join(hostDir, "modules-load.d", "uinput.conf")))
	if modules != "uinput" {
		t.Fatalf("modules-load.d must load uinput, got %q", modules)
	}
	pkgbuild := readFile(t, filepath.Join(hostDir, "arch", "PKGBUILD"))
	for _, dep := range []string{"ydotool", "xdotool", "libei"} {
		if strings.Contains(pkgbuild, dep) {
			t.Fatalf("PKGBUILD must not depend on %s", dep)
		}
	}
	if !strings.Contains(pkgbuild, "makedepends=('go>=1.26.8' 'gcc' 'cmake')") {
		t.Fatal("PKGBUILD must require the supported Go toolchain and core/gcc")
	}
	if !strings.Contains(pkgbuild, "-buildvcs=false") {
		t.Fatal("PKGBUILD must disable VCS stamping so extra/go can build outside a trusted git tree")
	}
	if strings.Contains(pkgbuild, "go test ./...") {
		t.Fatal("PKGBUILD check() must not run go test ./... from host/; that walks arch/pkg during makepkg")
	}
	if !strings.Contains(pkgbuild, "go test ./internal/... ./cmd/...") {
		t.Fatal("PKGBUILD check() must test ./internal/... and ./cmd/...")
	}
	control := readFile(t, filepath.Join(hostDir, "debian", "control"))
	for _, dep := range []string{"ydotool", "xdotool", "libei"} {
		if strings.Contains(control, dep) {
			t.Fatalf("debian control must not depend on %s", dep)
		}
	}
	if !strings.Contains(control, "Depends: @SHLIBS@, bluez, dbus, kmod, udev, qt6-qpa-plugins, qt6-wayland, hicolor-icon-theme") {
		t.Fatal("debian control must derive ABI dependencies and install Bluetooth, uinput and Qt platform support")
	}
	postinst := readFile(t, filepath.Join(hostDir, "debian", "postinst"))
	if !strings.Contains(postinst, "udevadm") {
		t.Fatal("debian postinst must reload udev")
	}
}

func TestDebianMaintainerLifecycle(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("requires a POSIX shell")
	}
	hostDir := testHostDir(t)
	for _, test := range []struct {
		script string
		action string
		want   string
	}{
		{"postinst", "configure", "modprobe uinput\nudevadm control --reload\nudevadm trigger --action=add --subsystem-match=misc --sysname-match=uinput\n"},
		{"postinst", "abort-upgrade", ""},
		{"postinst", "abort-remove", ""},
		{"postrm", "remove", "udevadm control --reload\nudevadm trigger --action=add --subsystem-match=misc --sysname-match=uinput\n"},
		{"postrm", "purge", "udevadm control --reload\nudevadm trigger --action=add --subsystem-match=misc --sysname-match=uinput\n"},
		{"postrm", "upgrade", ""},
		{"postrm", "failed-upgrade", ""},
	} {
		t.Run(test.script+"/"+test.action, func(t *testing.T) {
			for _, status := range []string{"0", "1"} {
				t.Run("helper-exit-"+status, func(t *testing.T) {
					dir := t.TempDir()
					logPath := filepath.Join(dir, "calls")
					for _, helper := range []string{"modprobe", "udevadm"} {
						body := "#!/bin/sh\nprintf '%s %s\\n' '" + helper + "' \"$*\" >> \"$CALL_LOG\"\nexit " + status + "\n"
						if err := os.WriteFile(filepath.Join(dir, helper), []byte(body), 0o755); err != nil {
							t.Fatal(err)
						}
					}
					cmd := exec.Command("sh", filepath.Join(hostDir, "debian", test.script), test.action)
					cmd.Env = append(os.Environ(), "PATH="+dir, "CALL_LOG="+logPath)
					if output, err := cmd.CombinedOutput(); err != nil {
						t.Fatalf("maintainer script failed without a live udev/kernel: %v\n%s", err, output)
					}
					calls, err := os.ReadFile(logPath)
					if err != nil && !os.IsNotExist(err) {
						t.Fatal(err)
					}
					if string(calls) != test.want {
						t.Fatalf("calls = %q, want %q", calls, test.want)
					}
				})
			}
		})
	}
}

func testHostDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found from working directory")
		}
		dir = parent
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
