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
	if Opts.DisableLockOrderDetection {
		return
	}
	gid := goid.Get()
	l.mu.Lock()
	for b, bs := range l.cur {
		if b == p {
			if bs.gid == gid {
				Opts.mu.Lock()
				fmt.Fprintln(Opts.LogBuf, header, "Recursive locking:")
				fmt.Fprintf(Opts.LogBuf, "current goroutine %d lock %p\n", gid, b)
				printStack(Opts.LogBuf, stack)
				fmt.Fprintln(Opts.LogBuf, "Previous place where the lock was grabbed (same goroutine)")
				printStack(Opts.LogBuf, bs.stack)
				l.other(p)
				if buf, ok := Opts.LogBuf.(*bufio.Writer); ok {
					buf.Flush()
				}
				Opts.mu.Unlock()
				Opts.OnPotentialDeadlock()
			}
			continue
		}
		if bs.gid != gid { // We want locks taken in the same goroutine only.
			continue
		}
		if s, ok := l.order[beforeAfter{p, b}]; ok {
			Opts.mu.Lock()
			fmt.Fprintln(Opts.LogBuf, header, "Inconsistent locking. saw this ordering in one goroutine:")
			fmt.Fprintln(Opts.LogBuf, "happened before")
			printStack(Opts.LogBuf, s.before)
			fmt.Fprintln(Opts.LogBuf, "happened after")
			printStack(Opts.LogBuf, s.after)
			fmt.Fprintln(Opts.LogBuf, "in another goroutine: happened before")
			printStack(Opts.LogBuf, bs.stack)
			fmt.Fprintln(Opts.LogBuf, "happened after")
			printStack(Opts.LogBuf, stack)
			l.other(p)
			fmt.Fprintln(Opts.LogBuf)
			if buf, ok := Opts.LogBuf.(*bufio.Writer); ok {
				buf.Flush()
			}
			Opts.mu.Unlock()
			Opts.OnPotentialDeadlock()
		}
		l.order[beforeAfter{b, p}] = ss{bs.stack, stack}
		if len(l.order) == Opts.MaxMapSize { // Reset the map to keep memory footprint bounded.
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
func (l *lockOrder) other(ptr interface{}) {
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
	fmt.Fprintln(Opts.LogBuf, "Other goroutines holding locks:")
	for k, pp := range l.cur {
		if k == ptr {
			continue
		}
		fmt.Fprintf(Opts.LogBuf, "goroutine %v lock %p\n", pp.gid, k)
		printStack(Opts.LogBuf, pp.stack)
	}
	fmt.Fprintln(Opts.LogBuf)
}

const header = "POTENTIAL DEADLOCK:"
