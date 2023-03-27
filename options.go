package deadlock

import (
	"bufio"
	"io"
	"os"
	"sync"
	"time"
)

var optsLock sync.RWMutex

type Options struct {
	// Waiting for a lock for longer than DeadlockTimeout is considered a deadlock.
	// Ignored if DeadlockTimeout <= 0.
	DeadlockTimeout time.Duration
	// OnPotentialDeadlock is called each time a potential deadlock is detected -- either based on
	// lock order or on lock wait time.
	OnPotentialDeadlock func()
	// Sets the maximum size of the map that tracks lock ordering.
	// Setting this to zero or lower disables tracking of lock order.
	MaxMapSize int
	// Will dump stacktraces of all goroutines when inconsistent locking is detected.
	PrintAllCurrentGoroutines bool
	// Will print deadlock info to log buffer.
	LogBuf io.Writer
}

// WriteLocked calls the given function with Opts locked for writing.
func (opts *Options) WriteLocked(fn func()) {
	optsLock.Lock()
	defer optsLock.Unlock()
	fn()
}

// ReadLocked calls the given function with Opts locked for reading.
func (opts *Options) ReadLocked(fn func()) {
	optsLock.RLock()
	defer optsLock.RUnlock()
	fn()
}

// Write implements io.Writer for Options.
func (opts *Options) Write(b []byte) (int, error) {
	if opts.LogBuf != nil {
		return opts.LogBuf.Write(b)
	}
	return 0, nil
}

// Flush will flush the LogBuf if it is a *bufio.Writer
func (opts *Options) Flush() error {
	if opts.LogBuf != nil {
		if buf, ok := opts.LogBuf.(*bufio.Writer); ok {
			return buf.Flush()
		}
	}
	return nil
}

// PotentialDeadlock calls OnPotentialDeadlock if it is set, or panics if not.
func (opts *Options) PotentialDeadlock() {
	if opts.OnPotentialDeadlock == nil {
		panic("deadlock detected")
	}
	opts.OnPotentialDeadlock()
}

// Opts control how deadlock detection behaves.
// To safely read or change options during runtime, use Opts.ReadLocked() and Opts.WriteLocked()
var Opts = Options{
	DeadlockTimeout: time.Second * 30,
	MaxMapSize:      1024 * 64,
	LogBuf:          os.Stderr,
}
