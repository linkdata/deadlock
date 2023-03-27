package deadlock

import (
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func restore() func() {
	var prevOpts Options
	Opts.ReadLocked(func() { prevOpts = Opts })
	return func() {
		Opts.WriteLocked(func() { Opts = prevOpts })
	}
}

func spinWait(t *testing.T, addr *uint32, want uint32) {
	for waited := 0; waited < 1000; waited++ {
		if atomic.LoadUint32(addr) == want {
			break
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(time.Millisecond * 10)
	if got := atomic.LoadUint32(addr); got != want {
		t.Fatal("expected 1 deadlock, detected", got)
	}
}

func randomWait(limit int) {
	if n := rand.Intn(limit); n > 0 {
		time.Sleep(time.Millisecond * time.Duration(n))
	} else {
		runtime.Gosched()
	}
}

func maybeLock(l sync.Locker, load *int32) bool {
	if rand.Intn(2) == 0 {
		return false
	}
	atomic.AddInt32(load, 1)
	l.Lock()
	return true
}

func doUnLock(l sync.Locker, load *int32) {
	l.Unlock()
	atomic.AddInt32(load, -1)
}

func TestNoDeadlocks(t *testing.T) {
	defer restore()()
	const timeout = time.Second * 10
	Opts.WriteLocked(func() {
		Opts.DeadlockTimeout = timeout
		Opts.MaxMapSize = 1
	})
	var a DeadlockRWMutex
	var b DeadlockMutex
	var c DeadlockRWMutex
	var wg sync.WaitGroup
	var load int32
	const wantedLoad = 50
	for i := runtime.NumCPU() * wantedLoad; i > 0 && atomic.LoadInt32(&load) < wantedLoad; i-- {
		wg.Add(1)
		go func() {
			defer wg.Done()
			func() {
				if maybeLock(&a, &load) {
					defer doUnLock(&a, &load)
				} else if maybeLock(a.RLocker(), &load) {
					defer doUnLock(a.RLocker(), &load)
				}
				func() {
					if maybeLock(&b, &load) {
						defer doUnLock(&b, &load)
					}
					func() {
						if maybeLock(&c, &load) {
							defer doUnLock(&c, &load)
						} else if maybeLock(c.RLocker(), &load) {
							defer doUnLock(c.RLocker(), &load)
						}
						randomWait(2)
					}()
				}()
			}()
		}()
	}
	ch := make(chan struct{})
	go func() {
		defer close(ch)
		wg.Wait()
	}()
	select {
	case <-ch:
	case <-time.After(timeout):
		t.Error("timeout waiting for load test to finish")
	}
}

func TestLockOrder(t *testing.T) {
	defer restore()()
	var deadlocks uint32
	Opts.WriteLocked(func() {
		Opts.DeadlockTimeout = 0
		Opts.OnPotentialDeadlock = func() {
			atomic.AddUint32(&deadlocks, 1)
		}
	})

	var a DeadlockRWMutex
	var b DeadlockMutex

	go func() {
		a.Lock()
		b.Lock()
		runtime.Gosched()
		b.Unlock()
		a.Unlock()
	}()
	spinWait(t, &deadlocks, 0)

	go func() {
		b.Lock()
		a.RLock()
		runtime.Gosched()
		a.RUnlock()
		b.Unlock()
	}()
	spinWait(t, &deadlocks, 1)
}

func TestHardDeadlock(t *testing.T) {
	defer restore()()
	var deadlocks uint32
	Opts.WriteLocked(func() {
		Opts.MaxMapSize = 0
		Opts.PrintAllCurrentGoroutines = true
		Opts.DeadlockTimeout = time.Millisecond * 20
		Opts.OnPotentialDeadlock = func() {
			atomic.AddUint32(&deadlocks, 1)
		}
	})
	var mu DeadlockMutex
	mu.Lock()
	ch := make(chan struct{})
	go func() {
		defer close(ch)
		mu.Lock()
		defer mu.Unlock()
	}()
	spinWait(t, &deadlocks, 1)
	mu.Unlock()
	select {
	case <-ch:
	case <-time.After(time.Millisecond * 100):
		t.Error("timeout waiting for deadlock to resolve")
	}
}

func TestRWMutex(t *testing.T) {
	defer restore()()
	var deadlocks uint32
	Opts.WriteLocked(func() {
		Opts.DeadlockTimeout = time.Millisecond * 20
		Opts.OnPotentialDeadlock = func() {
			atomic.AddUint32(&deadlocks, 1)
		}
	})
	var a DeadlockRWMutex

	a.Lock()
	go func() {
		a.Lock()
		defer a.Unlock()
	}()
	spinWait(t, &deadlocks, 1)

	ch := make(chan struct{})
	locker := a.RLocker()
	go func() {
		defer close(ch)
		locker.Lock()
		defer locker.Unlock()
	}()
	spinWait(t, &deadlocks, 2)
	a.Unlock()

	select {
	case <-ch:
	case <-time.After(time.Millisecond * 100):
		t.Error("timeout waiting for deadlock to resolve")
	}
}

func TestLockDuplicate(t *testing.T) {
	defer restore()()
	var deadlocks uint32
	Opts.WriteLocked(func() {
		Opts.DeadlockTimeout = 0
		Opts.OnPotentialDeadlock = func() {
			atomic.AddUint32(&deadlocks, 1)
		}
	})
	var a DeadlockRWMutex
	var b DeadlockMutex
	go func() {
		a.RLock()
		a.Lock()
		a.RUnlock()
		a.Unlock()
	}()
	go func() {
		b.Lock()
		b.Lock()
		runtime.Gosched()
		b.Unlock()
		b.Unlock()
	}()
	spinWait(t, &deadlocks, 2)
}
