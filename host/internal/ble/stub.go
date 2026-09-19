//go:build !linux && !windows

package ble

import "fmt"

func Start(hooks Hooks, localName string) Peripheral {
	_ = hooks
	_ = localName
	return Unavailable(fmt.Errorf("Bluetooth LE host is Linux and Windows only"))
}
