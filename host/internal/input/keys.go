package input

import "errors"

type keyState struct {
	held []uint16
}

func (s *keyState) tap(keys []uint16, emit func(uint16, bool) error) (err error) {
	if err = s.release(emit); err != nil {
		return err
	}
	defer func() { err = errors.Join(err, s.release(emit)) }()
	for _, key := range keys {
		// A failed write may have posted the key before failing to synchronize it.
		s.held = append(s.held, key)
		if err = emit(key, true); err != nil {
			return err
		}
	}
	return nil
}

func (s *keyState) release(emit func(uint16, bool) error) error {
	var result error
	for i := len(s.held) - 1; i >= 0; i-- {
		key := s.held[i]
		err := emit(key, false)
		if err != nil {
			result = errors.Join(result, err)
			err = emit(key, false)
			result = errors.Join(result, err)
		}
		if err == nil {
			s.held = append(s.held[:i], s.held[i+1:]...)
		}
	}
	return result
}
