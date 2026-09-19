package input

import (
	"fmt"
	"strings"
)

type SessionKind string

const (
	SessionWayland SessionKind = "wayland"
	SessionX11     SessionKind = "x11"
	SessionTTY     SessionKind = "tty"
	SessionUnknown SessionKind = "unknown"
)

const (
	HintLoadUinput = "load uinput: sudo modprobe uinput, then copy host/modules-load.d/uinput.conf to /etc/modules-load.d/stagewand-uinput.conf so the module loads at boot"
	HintUaccess    = "copy host/udev/70-stagewand-uinput.rules to /etc/udev/rules.d/, run sudo udevadm control --reload && sudo udevadm trigger --action=add --subsystem-match=misc, then log out of the graphical session and log in again so logind applies uaccess. Do not add the user to the input group"
	HintGraphical  = "run the host inside the graphical session so the compositor sees the virtual device"
)

type Probe struct {
	Device   string
	Exists   bool
	Writable bool
	OpenErr  string
	Session  SessionKind
	Desktop  string
	Display  string
	Hint     string
}

func SessionFromEnv(getenv func(string) string) (kind SessionKind, desktop, display string) {
	desktop = getenv("XDG_CURRENT_DESKTOP")
	if desktop == "" {
		desktop = getenv("DESKTOP_SESSION")
	}
	session := strings.ToLower(strings.TrimSpace(getenv("XDG_SESSION_TYPE")))
	waylandDisplay := getenv("WAYLAND_DISPLAY")
	xdisplay := getenv("DISPLAY")
	switch session {
	case "wayland":
		kind = SessionWayland
		display = waylandDisplay
	case "x11":
		kind = SessionX11
		display = xdisplay
	case "tty", "unspecified":
		kind = SessionTTY
	default:
		switch {
		case waylandDisplay != "":
			kind = SessionWayland
			display = waylandDisplay
		case xdisplay != "":
			kind = SessionX11
			display = xdisplay
		default:
			kind = SessionUnknown
		}
	}
	if kind == SessionWayland && display == "" {
		display = waylandDisplay
	}
	if kind == SessionX11 && display == "" {
		display = xdisplay
	}
	return kind, desktop, display
}

func (p Probe) Ready() (bool, string) {
	if p.Writable {
		return true, p.Summary()
	}
	if p.Hint != "" {
		return false, p.Hint
	}
	return false, "input injection unavailable"
}

func (p Probe) Summary() string {
	parts := []string{"uinput ready"}
	if p.Session != "" && p.Session != SessionUnknown {
		parts = append(parts, string(p.Session))
	}
	if p.Desktop != "" {
		parts = append(parts, p.Desktop)
	}
	if p.Display != "" {
		parts = append(parts, p.Display)
	}
	return strings.Join(parts, ", ")
}

func (p Probe) Report() string {
	var b strings.Builder
	fmt.Fprintf(&b, "device:    %s\n", p.Device)
	fmt.Fprintf(&b, "exists:    %t\n", p.Exists)
	fmt.Fprintf(&b, "writable:  %t\n", p.Writable)
	if p.OpenErr != "" {
		fmt.Fprintf(&b, "open:      %s\n", p.OpenErr)
	}
	fmt.Fprintf(&b, "session:   %s\n", p.Session)
	fmt.Fprintf(&b, "desktop:   %s\n", p.Desktop)
	fmt.Fprintf(&b, "display:   %s\n", p.Display)
	fmt.Fprintf(&b, "hint:      %s\n", p.Hint)
	return b.String()
}

func FinishProbe(p Probe) Probe {
	switch {
	case !p.Exists:
		p.Hint = HintLoadUinput
	case !p.Writable:
		p.Hint = HintUaccess
	case p.Session == SessionTTY || p.Session == SessionUnknown:
		p.Hint = p.Summary() + "; " + HintGraphical
	default:
		p.Hint = p.Summary()
	}
	return p
}
