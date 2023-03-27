package deadlock

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	"github.com/petermattis/goid"
)

// A DeadlockMutex is a drop-in replacement for sync.Mutex.
type DeadlockMutex struct {
	mu sync.Mutex
}

// Lock locks the mutex.
// If the lock is already in use, the calling goroutine
// blocks until the mutex is available.
//
// Logs potential deadlocks to Opts.LogBuf,
// calling Opts.OnPotentialDeadlock on each occasion.
func (m *DeadlockMutex) Lock() {
	lock(m.mu.Lock, m)
}

// Unlock unlocks the mutex.
// It is a run-time error if m is not locked on entry to Unlock.
//
// A locked Mutex is not associated with a particular goroutine.
// It is allowed for one goroutine to lock a Mutex and then
// arrange for another goroutine to unlock it.
func (m *DeadlockMutex) Unlock() {
	m.mu.Unlock()
	lo.postUnlock(m)
}

// An DeadlockRWMutex is a drop-in replacement for sync.RWMutex.
type DeadlockRWMutex struct {
	mu sync.RWMutex
}

// Lock locks rw for writing.
// If the lock is already locked for reading or writing,
// Lock blocks until the lock is available.
// To ensure that the lock eventually becomes available,
// a blocked Lock call excludes new readers from acquiring
// the lock.
//
// Logs potential deadlocks to Opts.LogBuf,
// calling Opts.OnPotentialDeadlock on each occasion.
func (m *DeadlockRWMutex) Lock() {
	lock(m.mu.Lock, m)
}

// Unlock unlocks the mutex for writing.  It is a run-time error if rw is
// not locked for writing on entry to Unlock.
//
// As with Mutexes, a locked RWMutex is not associated with a particular
// goroutine.  One goroutine may RLock (Lock) an RWMutex and then
// arrange for another goroutine to RUnlock (Unlock) it.
func (m *DeadlockRWMutex) Unlock() {
	m.mu.Unlock()
	lo.postUnlock(m)
}

// RLock locks the mutex for reading.
//
// Logs potential deadlocks to Opts.LogBuf,
// calling Opts.OnPotentialDeadlock on each occasion.
func (m *DeadlockRWMutex) RLock() {
	lock(m.mu.RLock, m)
}

// RUnlock undoes a single RLock call;
// it does not affect other simultaneous readers.
// It is a run-time error if rw is not locked for reading
// on entry to RUnlock.
func (m *DeadlockRWMutex) RUnlock() {
	m.mu.RUnlock()
	lo.postUnlock(m)
}

type rlocker DeadlockRWMutex

func (r *rlocker) Lock()   { (*DeadlockRWMutex)(r).RLock() }
func (r *rlocker) Unlock() { (*DeadlockRWMutex)(r).RUnlock() }

// RLocker returns a Locker interface that implements
// the Lock and Unlock methods by calling RLock and RUnlock.
func (m *DeadlockRWMutex) RLocker() sync.Locker {
	return (*rlocker)(m)
}

func lock(lockFn func(), ptr interface{}) {
	var opts Options
	Opts.ReadLocked(func() { opts = Opts })
	gid := goid.Get()
	stack := callers(1)
	lo.preLock(gid, stack, ptr)
	if opts.DeadlockTimeout > 0 {
		ch := make(chan struct{})
		defer close(ch)
		go func() {
			for {
				t := time.NewTimer(opts.DeadlockTimeout)
				defer t.Stop() // This runs after the closure finishes, but it's OK.
				select {
				case <-t.C:
					lo.mu.Lock()
					prev, ok := lo.cur[ptr]
					if !ok {
						lo.mu.Unlock()
						break // Nobody seems to be holding the lock, try again.
					}
					optsLock.Lock()
					fmt.Fprintln(&opts, header)
					fmt.Fprintln(&opts, "Previous place where the lock was grabbed")
					fmt.Fprintf(&opts, "goroutine %v lock %p\n", prev.gid, ptr)
					printStack(&opts, prev.stack)
					fmt.Fprintln(&opts, "Have been trying to lock it again for more than", opts.DeadlockTimeout)
					fmt.Fprintf(&opts, "goroutine %v lock %p\n", gid, ptr)
					printStack(&opts, stack)
					stacks := stacks()
					grs := bytes.Split(stacks, []byte("\n\n"))
					for _, g := range grs {
						if goid.ExtractGID(g) == prev.gid {
							fmt.Fprintln(&opts, "Here is what goroutine", prev.gid, "doing now")
							opts.Write(g)
							fmt.Fprintln(&opts)
						}
					}
					lo.other(&opts, ptr)
					if opts.PrintAllCurrentGoroutines {
						fmt.Fprintln(&opts, "All current goroutines:")
						opts.Write(stacks)
					}
					fmt.Fprintln(&opts)
					opts.Flush()
					optsLock.Unlock()
					lo.mu.Unlock()
					opts.OnPotentialDeadlock()
					<-ch
					return
				case <-ch:
					return
				}
			}
		}()
	}
	lockFn()
	lo.postLock(gid, stack, ptr)
}
