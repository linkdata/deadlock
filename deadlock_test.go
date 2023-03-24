package deadlock

import (
	"log"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNoDeadlocks(t *testing.T) {
	defer restore()()
	Opts.DeadlockTimeout = time.Millisecond * 5000
	var a DeadlockRWMutex
	var b DeadlockMutex
	var c DeadlockRWMutex
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < 5; k++ {
				func() {
					a.Lock()
					defer a.Unlock()
					func() {
						b.Lock()
						defer b.Unlock()
						func() {
							c.RLock()
							defer c.RUnlock()
							time.Sleep(time.Duration((1000 + rand.Intn(1000))) * time.Millisecond / 200)
						}()
					}()
				}()
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < 5; k++ {
				func() {
					a.RLock()
					defer a.RUnlock()
					func() {
						b.Lock()
						defer b.Unlock()
						func() {
							c.Lock()
							defer c.Unlock()
							time.Sleep(time.Duration((1000 + rand.Intn(1000))) * time.Millisecond / 200)
						}()
					}()
				}()
			}
		}()
	}
	wg.Wait()
}

func TestLockOrder(t *testing.T) {
	defer restore()()
	Opts.DeadlockTimeout = 0
	var deadlocks uint32
	Opts.OnPotentialDeadlock = func() {
		atomic.AddUint32(&deadlocks, 1)
	}
	var a DeadlockRWMutex
	var b DeadlockMutex
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.Lock()
		b.Lock()
		runtime.Gosched()
		b.Unlock()
		a.Unlock()
	}()
	wg.Wait()
	wg.Add(1)
	go func() {
		defer wg.Done()
		b.Lock()
		a.RLock()
		runtime.Gosched()
		a.RUnlock()
		b.Unlock()
	}()
	wg.Wait()
	if atomic.LoadUint32(&deadlocks) != 1 {
		t.Fatalf("expected 1 deadlock, detected %d", deadlocks)
	}
}

func TestHardDeadlock(t *testing.T) {
	defer restore()()
	Opts.DisableLockOrderDetection = true
	Opts.DeadlockTimeout = time.Millisecond * 20
	var deadlocks uint32
	Opts.OnPotentialDeadlock = func() {
		atomic.AddUint32(&deadlocks, 1)
	}
	var mu DeadlockMutex
	mu.Lock()
	ch := make(chan struct{})
	go func() {
		defer close(ch)
		mu.Lock()
		defer mu.Unlock()
	}()
	select {
	case <-ch:
	case <-time.After(time.Millisecond * 100):
	}
	if atomic.LoadUint32(&deadlocks) != 1 {
		t.Fatalf("expected 1 deadlock, detected %d", deadlocks)
	}
	log.Println("****************")
	mu.Unlock()
	<-ch
}

func TestRWMutex(t *testing.T) {
	defer restore()()
	Opts.DeadlockTimeout = time.Millisecond * 20
	var deadlocks uint32
	Opts.OnPotentialDeadlock = func() {
		atomic.AddUint32(&deadlocks, 1)
	}
	var a DeadlockRWMutex
	a.RLock()
	go func() {
		// We detect a potential deadlock here.
		a.Lock()
		defer a.Unlock()
	}()
	time.Sleep(time.Millisecond * 100) // We want the Lock call to happen.
	ch := make(chan struct{})
	go func() {
		// We detect a potential deadlock here.
		defer close(ch)
		a.RLock()
		defer a.RUnlock()
	}()
	select {
	case <-ch:
		t.Fatal("expected a timeout")
	case <-time.After(time.Millisecond * 50):
	}
	a.RUnlock()
	if atomic.LoadUint32(&deadlocks) != 2 {
		t.Fatalf("expected 2 deadlocks, detected %d", deadlocks)
	}
	<-ch
}

func restore() func() {
	opts := Opts
	return func() {
		Opts = opts
	}
}

func TestLockDuplicate(t *testing.T) {
	defer restore()()
	Opts.DeadlockTimeout = 0
	var deadlocks uint32
	Opts.OnPotentialDeadlock = func() {
		atomic.AddUint32(&deadlocks, 1)
	}
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
	time.Sleep(time.Second * 1)
	if atomic.LoadUint32(&deadlocks) != 2 {
		t.Fatalf("expected 2 deadlocks, detected %d", deadlocks)
	}
}
