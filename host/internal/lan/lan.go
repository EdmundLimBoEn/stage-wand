package lan

import (
	"net"
	"strings"
)

func IPv4() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	var addresses []struct{ name, ip string }
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip, ok := addr.(*net.IPNet)
			if !ok || ip.IP.To4() == nil {
				continue
			}
			addresses = append(addresses, struct{ name, ip string }{iface.Name, ip.IP.String()})
		}
	}
	for _, prefix := range []string{"en0", "eth0", "wlan0", "en", "eth", "wl"} {
		for _, address := range addresses {
			if address.name == prefix || strings.HasPrefix(address.name, prefix) {
				return address.ip
			}
		}
	}
	if len(addresses) > 0 {
		return addresses[0].ip
	}
	return ""
}
