//go:build go1.18
// +build go1.18

package deadlock

import (
	"sync/atomic"
	"testing"
)

func TestDeadlockMutex_TryLock(t *testing.T) {
	defer restore()()
	var deadlocks uint32
	Opts.WriteLocked(func() {
		Opts.DeadlockTimeout = 0
		Opts.OnPotentialDeadlock = func() {
			atomic.AddUint32(&deadlocks, 1)
		}
	})

	var a DeadlockRWMutex
	if a.TryLock() {
		if a.TryRLock() {
			t.Fatal("expected TryRLock to fail")
		}
	} else {
		t.Fatal("expected TryLock to succeed")
	}

	var b DeadlockMutex
	if b.TryLock() {
		if b.TryLock() {
			t.Fatal("expected TryLock to fail")
		}
	} else {
		t.Fatal("expected TryLock to succeed")
	}

	if deadlocks != 0 {
		t.Fatal("got", deadlocks, "deadlocks, expected none")
	}
}
