//go:build !linux && !windows

package input

import (
	"fmt"
	"runtime"

	"github.com/EdmundLimBoEn/stage-wand/host/internal/protocol"
)

type Unsupported struct{}

func Open() (Injector, error) {
	return nil, fmt.Errorf("input injection is not implemented on %s", runtime.GOOS)
}

func OpenOrError() (Injector, error) { return Open() }

func (Unsupported) Apply(protocol.Command) error { return nil }
func (Unsupported) Ready() (bool, string) {
	return false, fmt.Sprintf("input injection is not implemented on %s", runtime.GOOS)
}
func (Unsupported) Close() error { return nil }
