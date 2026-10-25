package main

import (
	"errors"
	"strings"
	"testing"
)

type fakeCloser struct{ err error }

func (f fakeCloser) Close() error { return f.err }

func TestCloseDB(t *testing.T) {
	var err error
	closeDB(fakeCloser{}, &err)
	if err != nil {
		t.Errorf("clean close: err = %v, want nil", err)
	}

	err = nil
	closeDB(fakeCloser{err: errors.New("checkpoint failed")}, &err)
	if err == nil || !strings.Contains(err.Error(), "close database: checkpoint failed") {
		t.Errorf("failed close on success path: err = %v", err)
	}

	runErr := errors.New("index failed")
	err = runErr
	closeDB(fakeCloser{err: errors.New("checkpoint failed")}, &err)
	if !errors.Is(err, runErr) || !strings.Contains(err.Error(), "checkpoint failed") {
		t.Errorf("both errors must survive: err = %v", err)
	}
}
