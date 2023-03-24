package deadlock

import (
	"bufio"
	"fmt"
	"sync"

	"github.com/petermattis/goid"
)

type lockOrder struct {
	mu    sync.Mutex
	cur   map[interface{}]stackGID // stacktraces + gids for the locks currently taken.
	order map[beforeAfter]ss       // expected order of locks.
}

type stackGID struct {
	stack []uintptr
	gid   int64
}

type beforeAfter struct {
	before interface{}
	after  interface{}
}

type ss struct {
	before []uintptr
	after  []uintptr
}

var lo = newLockOrder()

func newLockOrder() *lockOrder {
	return &lockOrder{
		cur:   map[interface{}]stackGID{},
		order: map[beforeAfter]ss{},
	}
}

func (l *lockOrder) postLock(stack []uintptr, p interface{}) {
	gid := goid.Get()
	l.mu.Lock()
	l.cur[p] = stackGID{stack, gid}
	l.mu.Unlock()
}

func (l *lockOrder) preLock(stack []uintptr, p interface{}) {
	var opts Options
	Opts.Read(func() { opts = Opts })
	if opts.DisableLockOrderDetection {
		return
	}
	gid := goid.Get()
	l.mu.Lock()
	for b, bs := range l.cur {
		if b == p {
			if bs.gid == gid {
				optsLock.Lock()
				fmt.Fprintln(opts.LogBuf, header, "Recursive locking:")
				fmt.Fprintf(opts.LogBuf, "current goroutine %d lock %p\n", gid, b)
				printStack(opts.LogBuf, stack)
				fmt.Fprintln(opts.LogBuf, "Previous place where the lock was grabbed (same goroutine)")
				printStack(opts.LogBuf, bs.stack)
				l.other(&opts, p)
				if buf, ok := opts.LogBuf.(*bufio.Writer); ok {
					buf.Flush()
				}
				optsLock.Unlock()
				opts.OnPotentialDeadlock()
			}
			continue
		}
		if bs.gid != gid { // We want locks taken in the same goroutine only.
			continue
		}
		if s, ok := l.order[beforeAfter{p, b}]; ok {
			optsLock.Lock()
			fmt.Fprintln(opts.LogBuf, header, "Inconsistent locking. saw this ordering in one goroutine:")
			fmt.Fprintln(opts.LogBuf, "happened before")
			printStack(opts.LogBuf, s.before)
			fmt.Fprintln(opts.LogBuf, "happened after")
			printStack(opts.LogBuf, s.after)
			fmt.Fprintln(opts.LogBuf, "in another goroutine: happened before")
			printStack(opts.LogBuf, bs.stack)
			fmt.Fprintln(opts.LogBuf, "happened after")
			printStack(opts.LogBuf, stack)
			l.other(&opts, p)
			fmt.Fprintln(opts.LogBuf)
			if buf, ok := opts.LogBuf.(*bufio.Writer); ok {
				buf.Flush()
			}
			optsLock.Unlock()
			opts.OnPotentialDeadlock()
		}
		l.order[beforeAfter{b, p}] = ss{bs.stack, stack}
		if len(l.order) == opts.MaxMapSize { // Reset the map to keep memory footprint bounded.
			l.order = map[beforeAfter]ss{}
		}
	}
	l.mu.Unlock()
}

func (l *lockOrder) postUnlock(p interface{}) {
	l.mu.Lock()
	delete(l.cur, p)
	l.mu.Unlock()
}

// Under lo.mu Locked.
func (l *lockOrder) other(opts *Options, ptr interface{}) {
	empty := true
	for k := range l.cur {
		if k == ptr {
			continue
		}
		empty = false
	}
	if empty {
		return
	}
	fmt.Fprintln(opts.LogBuf, "Other goroutines holding locks:")
	for k, pp := range l.cur {
		if k == ptr {
			continue
		}
		fmt.Fprintf(opts.LogBuf, "goroutine %v lock %p\n", pp.gid, k)
		printStack(opts.LogBuf, pp.stack)
	}
	fmt.Fprintln(opts.LogBuf)
}

const header = "POTENTIAL DEADLOCK:"
