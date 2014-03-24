package sync

import (
	"testing"
	"time"
)

func TestRunSuccess(t *testing.T) {
	timeout := 1 * time.Millisecond
	var ranOk bool
	ok := RunWithTimeout(timeout, func() {
		ranOk = true
	})
	if !ok || !ranOk {
		t.Fail()
	}
}

func TestRunTimeout(t *testing.T) {
	timeout := 1 * time.Millisecond
	var ranOk bool
	sig := make(chan struct{})
	ok := RunWithTimeout(timeout, func() {
		// Block until we timeout.
		<-sig
		ranOk = true
	})
	sig <- struct{}{}
	if ok || ranOk {
		t.Fail()
	}
}
