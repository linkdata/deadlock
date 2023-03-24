package deadlock

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/petermattis/goid"
)

// Opts control how deadlock detection behaves.
// Options are supposed to be set once at a startup (say, when parsing flags).
var Opts = struct {
	// Would disable lock order based deadlock detection if DisableLockOrderDetection == true.
	DisableLockOrderDetection bool
	// Waiting for a lock for longer than DeadlockTimeout is considered a deadlock.
	// Ignored if DeadlockTimeout <= 0.
	DeadlockTimeout time.Duration
	// OnPotentialDeadlock is called each time a potential deadlock is detected -- either based on
	// lock order or on lock wait time.
	OnPotentialDeadlock func()
	// Will keep MaxMapSize lock pairs (happens before // happens after) in the map.
	// The map resets once the threshold is reached.
	MaxMapSize int
	// Will dump stacktraces of all goroutines when inconsistent locking is detected.
	PrintAllCurrentGoroutines bool
	mu                        *sync.Mutex // Protects the LogBuf.
	// Will print deadlock info to log buffer.
	LogBuf io.Writer
}{
	DeadlockTimeout: time.Second * 30,
	OnPotentialDeadlock: func() {
		os.Exit(2)
	},
	MaxMapSize: 1024 * 64,
	mu:         &sync.Mutex{},
	LogBuf:     os.Stderr,
}

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
	stack := callers(1)
	lo.preLock(stack, ptr)
	if Opts.DeadlockTimeout <= 0 {
		lockFn()
	} else {
		ch := make(chan struct{})
		currentID := goid.Get()
		go func() {
			for {
				t := time.NewTimer(Opts.DeadlockTimeout)
				defer t.Stop() // This runs after the closure finishes, but it's OK.
				select {
				case <-t.C:
					lo.mu.Lock()
					prev, ok := lo.cur[ptr]
					if !ok {
						lo.mu.Unlock()
						break // Nobody seems to be holding the lock, try again.
					}
					Opts.mu.Lock()
					fmt.Fprintln(Opts.LogBuf, header)
					fmt.Fprintln(Opts.LogBuf, "Previous place where the lock was grabbed")
					fmt.Fprintf(Opts.LogBuf, "goroutine %v lock %p\n", prev.gid, ptr)
					printStack(Opts.LogBuf, prev.stack)
					fmt.Fprintln(Opts.LogBuf, "Have been trying to lock it again for more than", Opts.DeadlockTimeout)
					fmt.Fprintf(Opts.LogBuf, "goroutine %v lock %p\n", currentID, ptr)
					printStack(Opts.LogBuf, stack)
					stacks := stacks()
					grs := bytes.Split(stacks, []byte("\n\n"))
					for _, g := range grs {
						if goid.ExtractGID(g) == prev.gid {
							fmt.Fprintln(Opts.LogBuf, "Here is what goroutine", prev.gid, "doing now")
							Opts.LogBuf.Write(g)
							fmt.Fprintln(Opts.LogBuf)
						}
					}
					lo.other(ptr)
					if Opts.PrintAllCurrentGoroutines {
						fmt.Fprintln(Opts.LogBuf, "All current goroutines:")
						Opts.LogBuf.Write(stacks)
					}
					fmt.Fprintln(Opts.LogBuf)
					if buf, ok := Opts.LogBuf.(*bufio.Writer); ok {
						buf.Flush()
					}
					Opts.mu.Unlock()
					lo.mu.Unlock()
					Opts.OnPotentialDeadlock()
					<-ch
					return
				case <-ch:
					return
				}
			}
		}()
		lockFn()
		lo.postLock(stack, ptr)
		close(ch)
		return
	}
	lo.postLock(stack, ptr)
}
