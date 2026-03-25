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

type rwlocker interface {
	RLock()
	RUnlock()
}

func runlock(l rwlocker) {
	l.RUnlock()
}

func unlock(l sync.Locker) {
	l.Unlock()
}

func TestStarvedRLockMultipleReaders(t *testing.T) {
	defer restore()()
	var deadlocks uint32
	Opts.WriteLocked(func() {
		Opts.DeadlockTimeout = time.Millisecond * 20
		Opts.OnPotentialDeadlock = func() {
			atomic.AddUint32(&deadlocks, 1)
		}
	})

	var a RWMutex

	// Reader 1 holds RLock for the duration of the test.
	a.RLock()

	// Reader 2 briefly holds and releases RLock. Before the fix, this
	// corrupted the cur map: postLock overwrote reader 1's entry, then
	// postUnlock deleted it entirely even though reader 1 still held it.
	done := make(chan struct{})
	go func() {
		a.RLock()
		runlock(&a)
		close(done)
	}()
	<-done

	// Writer tries to Lock — blocks because reader 1 still holds RLock.
	go func() {
		a.Lock()
		defer a.Unlock()
	}()
	time.Sleep(time.Millisecond * 100)

	// Starved reader tries RLock — blocked by the pending writer.
	ch := make(chan struct{})
	go func() {
		defer close(ch)
		a.RLock()
		defer a.RUnlock()
	}()
	select {
	case <-ch:
		t.Fatal("expected a timeout")
	case <-time.After(time.Millisecond * 100):
	}

	if atomic.LoadUint32(&deadlocks) != 2 {
		t.Fatalf("expected 2 deadlocks, detected %d", deadlocks)
	}
	a.RUnlock()
	<-ch
}

// TestManyReadersFewWriters stresses the RWMutex tracking under high read
// concurrency with infrequent writers. Existing tests use at most ~10
// goroutines with a balanced reader/writer mix; real-world usage often has
// dozens of readers racing against a handful of writers. This exercises:
//   - the per-goroutine cur map ref-counting under heavy concurrent RLock/RUnlock,
//     where many goroutines simultaneously call postLock and postUnlock;
//   - lock-order detection with a large number of concurrent reader entries;
//   - timer pool contention when many DeadlockTimeout timers are live at once.
func TestManyReadersFewWriters(t *testing.T) {
	defer restore()()
	Opts.WriteLocked(func() {
		Opts.DeadlockTimeout = time.Millisecond * 5000
	})

	var mu RWMutex
	var wg sync.WaitGroup

	const numReaders = 100
	const numWriters = 3
	const readerIters = 50
	const writerIters = 10

	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < readerIters; k++ {
				mu.RLock()
				time.Sleep(time.Duration(rand.Intn(500)) * time.Microsecond)
				mu.RUnlock()
			}
		}()
	}

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < writerIters; k++ {
				mu.Lock()
				time.Sleep(time.Duration(rand.Intn(200)) * time.Microsecond)
				mu.Unlock()
				time.Sleep(time.Duration(rand.Intn(1000)) * time.Microsecond)
			}
		}()
	}

	wg.Wait()
}

// TestConcurrentLockOrderDetection verifies that lock-order violation detection
// works correctly under real goroutine contention. TestLockOrder runs its two
// goroutines sequentially (wg.Wait() between them), so the order map and cur map
// are only contested by one goroutine at a time. Here, many goroutines
// simultaneously call preLock, postLock, and postUnlock — all contending on
// lo.mu — while each one independently detects the same A→B vs B→A conflict.
// This stresses concurrent iteration of lo.cur, concurrent reads/writes to
// lo.order, and concurrent invocations of OnPotentialDeadlock.
func TestConcurrentLockOrderDetection(t *testing.T) {
	defer restore()()
	var deadlocks uint32
	Opts.WriteLocked(func() {
		Opts.DeadlockTimeout = 0
		Opts.OnPotentialDeadlock = func() {
			atomic.AddUint32(&deadlocks, 1)
		}
	})

	var a, b Mutex

	// Establish the A→B ordering in the lock-order map.
	a.Lock()
	b.Lock()
	unlock(&b)
	a.Unlock()

	// Launch many goroutines that all acquire B→A concurrently. Each one
	// triggers a violation in preLock when it tries to acquire A while holding
	// B. Because every goroutine acquires in the same order (B then A), they
	// cannot actually deadlock with each other.
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for k := 0; k < 10; k++ {
				b.Lock()
				a.Lock()
				unlock(&a)
				b.Unlock()
			}
		}()
	}

	close(start)
	wg.Wait()

	if d := atomic.LoadUint32(&deadlocks); d == 0 {
		t.Fatal("expected at least 1 lock-order violation, detected 0")
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

	aStart := make(chan struct{})
	aDone := make(chan struct{})
	go func() {
		defer close(aDone)
		a.RLock()
		close(aStart)
		a.Lock()
		unlock(&a)
	}()
	<-aStart
	spinWait(t, &deadlocks, 1)
	a.RUnlock()
	select {
	case <-aDone:
	case <-time.After(time.Millisecond * 100):
		t.Fatal("timeout waiting for recursive RWMutex test goroutine")
	}

	bStart := make(chan struct{})
	bDone := make(chan struct{})
	go func() {
		defer close(bDone)
		b.Lock()
		close(bStart)
		b.Lock()
		runtime.Gosched()
		b.Unlock()
	}()
	<-bStart
	spinWait(t, &deadlocks, 2)
	b.Unlock()
	select {
	case <-bDone:
	case <-time.After(time.Millisecond * 100):
		t.Fatal("timeout waiting for recursive Mutex test goroutine")
	}
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
