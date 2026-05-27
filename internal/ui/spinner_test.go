package ui

import (
	"errors"
	"testing"
)

func TestRunWithSpinnerPlainCallsFn(t *testing.T) {
	called := false
	err := RunWithSpinner("loading...", true, func() error {
		called = true
		return nil
	})
	if !called {
		t.Error("expected fn to be called in plain mode")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunWithSpinnerPassesError(t *testing.T) {
	expected := errors.New("api error")
	err := RunWithSpinner("loading...", true, func() error {
		return expected
	})
	if err != expected {
		t.Errorf("expected %v, got %v", expected, err)
	}
}
