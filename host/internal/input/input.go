package input

import "github.com/EdmundLimBoEn/stage-wand/host/internal/protocol"

type Injector interface {
	Apply(command protocol.Command) error
	Ready() (ok bool, detail string)
	Close() error
}

type LogFunc func(command protocol.Command)

type Logging struct {
	Log LogFunc
}

func (l Logging) Apply(command protocol.Command) error {
	if l.Log != nil {
		l.Log(command)
	}
	return nil
}

func (Logging) Ready() (bool, string) { return true, "dry-run (no input injection)" }
func (Logging) Close() error          { return nil }
