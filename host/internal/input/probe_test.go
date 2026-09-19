package input

import (
	"strings"
	"testing"
)

func TestSessionFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    SessionKind
		desktop string
		display string
	}{
		{
			name:    "hyprland",
			env:     map[string]string{"XDG_SESSION_TYPE": "wayland", "WAYLAND_DISPLAY": "wayland-1", "XDG_CURRENT_DESKTOP": "Hyprland"},
			want:    SessionWayland,
			desktop: "Hyprland",
			display: "wayland-1",
		},
		{
			name:    "sway",
			env:     map[string]string{"XDG_SESSION_TYPE": "wayland", "WAYLAND_DISPLAY": "wayland-0", "XDG_CURRENT_DESKTOP": "sway"},
			want:    SessionWayland,
			desktop: "sway",
			display: "wayland-0",
		},
		{
			name:    "i3 x11",
			env:     map[string]string{"XDG_SESSION_TYPE": "x11", "DISPLAY": ":0", "XDG_CURRENT_DESKTOP": "i3"},
			want:    SessionX11,
			desktop: "i3",
			display: ":0",
		},
		{
			name: "tty ssh",
			env:  map[string]string{"XDG_SESSION_TYPE": "tty"},
			want: SessionTTY,
		},
		{
			name:    "wayland display only",
			env:     map[string]string{"WAYLAND_DISPLAY": "wayland-0"},
			want:    SessionWayland,
			display: "wayland-0",
		},
		{
			name:    "x11 display only",
			env:     map[string]string{"DISPLAY": ":1"},
			want:    SessionX11,
			display: ":1",
		},
		{
			name: "empty",
			env:  map[string]string{},
			want: SessionUnknown,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, desktop, display := SessionFromEnv(func(key string) string { return tc.env[key] })
			if got != tc.want || desktop != tc.desktop || display != tc.display {
				t.Fatalf("got %s desktop=%q display=%q", got, desktop, display)
			}
		})
	}
}

func TestFinishProbeHints(t *testing.T) {
	missing := FinishProbe(Probe{Device: "/dev/uinput", Exists: false, Session: SessionWayland, Desktop: "Hyprland"})
	if missing.Hint != HintLoadUinput {
		t.Fatalf("missing module hint: %q", missing.Hint)
	}
	denied := FinishProbe(Probe{Device: "/dev/uinput", Exists: true, Writable: false, Session: SessionWayland})
	if denied.Hint != HintUaccess {
		t.Fatalf("uaccess hint: %q", denied.Hint)
	}
	if denied.Hint == HintLoadUinput {
		t.Fatal("permission failure must not tell the user only to modprobe")
	}
	ok, detail := FinishProbe(Probe{Device: "/dev/uinput", Exists: true, Writable: true, Session: SessionWayland, Desktop: "Hyprland", Display: "wayland-1"}).Ready()
	if !ok {
		t.Fatalf("expected ready, got %s", detail)
	}
	if detail != "uinput ready, wayland, Hyprland, wayland-1" {
		t.Fatalf("summary: %q", detail)
	}
	tty := FinishProbe(Probe{Device: "/dev/uinput", Exists: true, Writable: true, Session: SessionTTY})
	if !strings.Contains(tty.Hint, HintGraphical) {
		t.Fatalf("tty hint: %q", tty.Hint)
	}
}
