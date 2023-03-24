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
	// Will print deadlock info to log buffer.
	LogBuf io.Writer
}

// WriteLocked calls the given function with Opts locked for writing.
// Not needed unless you modify options while locks are being held.
func (opts *Options) WriteLocked(fn func()) {
	optsLock.Lock()
	defer optsLock.Unlock()
	fn()
}

// ReadLocked calls the given function with Opts locked for reading.
// Not needed unless you modify options while locks are being held.
func (opts *Options) ReadLocked(fn func()) {
	optsLock.RLock()
	defer optsLock.RUnlock()
	fn()
}

func (opts *Options) Write(b []byte) (int, error) {
	if opts.LogBuf != nil {
		return opts.LogBuf.Write(b)
	}
	return 0, nil
}

func (opts *Options) Flush() error {
	if opts.LogBuf != nil {
		if buf, ok := opts.LogBuf.(*bufio.Writer); ok {
			return buf.Flush()
		}
	}
	return nil
}

// Opts control how deadlock detection behaves.
// Options are supposed to be set once at a startup (say, when parsing flags).
var Opts = Options{
	DeadlockTimeout:     time.Second * 30,
	OnPotentialDeadlock: func() { os.Exit(2) },
	MaxMapSize:          1024 * 64,
	LogBuf:              os.Stderr,
}
