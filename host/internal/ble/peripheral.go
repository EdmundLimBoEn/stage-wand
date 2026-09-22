package ble

// Peripheral is the GATT adapter. Start returns a usable Peripheral even when
// Bluetooth is unavailable; Note reports the current transport status.
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
