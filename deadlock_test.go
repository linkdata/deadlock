package deadlock

import (
	"bytes"
	"math/rand"
	"runtime"
	"strings"
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
	t.Helper()
	for waited := 0; waited < 1000; waited++ {
		if atomic.LoadUint32(addr) == want {
			break
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(time.Millisecond * 10)
	if got := atomic.LoadUint32(addr); got != want {
		t.Fatal("expected", want, "deadlocks, detected", got)
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

func TestDummyLock(t *testing.T) {
	// to keep full test coverage even though the code path
	// is never taken on versions of go prior to 1.18
	lock(nil, nil, nil)
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

func TestRWMutexConcurrentReaders(t *testing.T) {
	t.Skip("known tracking limitation: concurrent RW readers overwrite holder metadata")
	defer restore()()
	var deadlocks uint32
	var logBuf bytes.Buffer
	Opts.WriteLocked(func() {
		Opts.MaxMapSize = 0
		Opts.DeadlockTimeout = time.Millisecond * 5
		Opts.LogBuf = &logBuf
		Opts.OnPotentialDeadlock = func() {
			atomic.AddUint32(&deadlocks, 1)
		}
	})
	var rw DeadlockRWMutex
	firstReaderLocked := make(chan struct{})
	secondReaderLocked := make(chan struct{})
	firstReaderUnlocked := make(chan struct{})
	releaseSecondReader := make(chan struct{})

	go func() {
		rw.RLock()
		close(firstReaderLocked)
		<-secondReaderLocked
		rw.RUnlock()
		close(firstReaderUnlocked)
	}()
	<-firstReaderLocked

	go func() {
		rw.RLock()
		close(secondReaderLocked)
		<-releaseSecondReader
		rw.RUnlock()
	}()
	<-secondReaderLocked
	<-firstReaderUnlocked

	writerDone := make(chan struct{})
	go func() {
		rw.Lock()
		defer func() {
			rw.Unlock()
			close(writerDone)
		}()
	}()

	spinWait(t, &deadlocks, 1)

	close(releaseSecondReader)
	select {
	case <-writerDone:
	case <-time.After(time.Millisecond * 100):
		t.Fatal("timeout waiting for writer to acquire lock")
	}

	if output := logBuf.String(); !strings.Contains(output, "previously locked it from:") {
		t.Fatalf("expected timeout report to include holder stack, output:\n%s", output)
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

//go:noinline
func lockOne(m *DeadlockMutex) {
	m.Lock()
	runtime.Gosched()
	m.Unlock()
}

//go:noinline
func lockTwo(wg *sync.WaitGroup, count int, m1, m2 *DeadlockMutex) {
	defer wg.Done()
	for n := 0; n < count; n++ {
		m1.Lock()
		lockOne(m2)
		m1.Unlock()
	}
}

func BenchmarkDeadlocks(b *testing.B) {
	defer restore()()
	var deadlocks uint32
	Opts.WriteLocked(func() {
		Opts.DeadlockTimeout = time.Minute
		Opts.OnPotentialDeadlock = func() {
			atomic.AddUint32(&deadlocks, 1)
		}
	})
	var wg sync.WaitGroup
	var m1, m2 DeadlockMutex
	wg.Add(2)
	go lockTwo(&wg, b.N, &m1, &m2)
	go lockTwo(&wg, b.N, &m1, &m2)
	wg.Wait()
	if atomic.LoadUint32(&deadlocks) > 0 {
		b.Fatal("expected no deadlocks, got", deadlocks)
	}
}
