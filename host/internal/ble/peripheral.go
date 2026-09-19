package ble

// Peripheral is the GATT adapter. Start advertises the Stage Wand service.
// Close and Kick are safe on a nil Peripheral.
type Peripheral interface {
	Close() error
	Kick()
	Note() string
}

type failed struct {
	err error
}

func (f failed) Close() error { return nil }
func (f failed) Kick()        {}
func (f failed) Note() string {
	if f.err == nil {
		return "unavailable"
	}
	return "unavailable (" + f.err.Error() + ")"
}

func Unavailable(err error) Peripheral {
	return failed{err: err}
}
