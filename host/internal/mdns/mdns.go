package mdns

import (
	"fmt"
	"os"

	"github.com/grandcat/zeroconf"
)

type Advertisement struct {
	server *zeroconf.Server
}

func Advertise(name string, port int) (*Advertisement, error) {
	if name == "" {
		host, err := os.Hostname()
		if err != nil || host == "" {
			name = "StageWand"
		} else {
			name = host
		}
	}
	server, err := zeroconf.Register(name, "_stagewand._tcp", "local.", port, []string{"txtvers=1"}, nil)
	if err != nil {
		return nil, fmt.Errorf("mDNS advertise: %w", err)
	}
	return &Advertisement{server: server}, nil
}

func (a *Advertisement) Close() {
	if a != nil && a.server != nil {
		a.server.Shutdown()
	}
}
