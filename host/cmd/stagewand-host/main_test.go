package main

import (
	"errors"
	"testing"

	"github.com/EdmundLimBoEn/stage-wand/host/internal/input"
	"github.com/EdmundLimBoEn/stage-wand/host/internal/protocol"
)

type failingInjector struct {
	input.Logging
	failure  error
	commands int
}

func (f *failingInjector) Apply(command protocol.Command) error {
	f.commands++
	if _, ok := command.(protocol.KeyPress); ok {
		return f.failure
	}
	return nil
}

func TestSelfTestReportsKeyboardFailure(t *testing.T) {
	failure := errors.New("SendInput rejected keyboard event")
	injector := &failingInjector{failure: failure}
	if err := applySelfTest(injector); !errors.Is(err, failure) {
		t.Fatalf("got %v", err)
	}
	if injector.commands != 4 {
		t.Fatalf("continued after failure: %d commands", injector.commands)
	}
}

func TestSelfTestPostsEveryCommand(t *testing.T) {
	count := 0
	err := applySelfTest(input.Logging{Log: func(protocol.Command) { count++ }})
	if err != nil || count != 5 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}
