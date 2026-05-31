package testutil

import (
	"testing"
	"time"
)

func WaitForEvent[E any](t *testing.T, ch <-chan E, timeout time.Duration, pred func(E) bool) E {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case ev := <-ch:
			if pred(ev) {
				return ev
			}
		case <-deadline:
			t.Fatal("timed out waiting for matching event")
			var zero E
			return zero
		}
	}
}
