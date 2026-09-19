package packaging_test

import (
	"os"
	"path/filepath"
	"runtime"
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
	if !strings.Contains(pkgbuild, "makedepends=('go' 'gcc')") {
		t.Fatal("PKGBUILD must build with extra/go and core/gcc")
	}
	if !strings.Contains(pkgbuild, "-buildvcs=false") {
		t.Fatal("PKGBUILD must disable VCS stamping so extra/go can build outside a trusted git tree")
	}
	readme := readFile(t, filepath.Join(hostDir, "..", "README.md"))
	if !strings.Contains(readme, "pacman -S --needed go git") {
		t.Fatal("README must show the Arch pacman install line")
	}
	if strings.Contains(readme, "usermod -aG input") {
		t.Fatal("README must not tell users to join the input group")
	}
	if !strings.Contains(readme, "uaccess") {
		t.Fatal("README must mention uaccess")
	}
	if !strings.Contains(readme, "apt install golang-go git") {
		t.Fatal("README must show the Debian install line")
	}
}

func testHostDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(file)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
