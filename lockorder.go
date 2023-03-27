package deadlock

import (
	"fmt"
	"sync"
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

func (l *lockOrder) postLock(gid int64, stack []uintptr, curMtx interface{}) {
	l.mu.Lock()
	l.cur[curMtx] = stackGID{stack, gid}
	l.mu.Unlock()
}

func (l *lockOrder) preLock(gid int64, stack []uintptr, curMtx interface{}) {
	var opts Options
	Opts.ReadLocked(func() { opts = Opts })
	if opts.MaxMapSize < 1 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for b, bs := range l.cur {
		if b == curMtx {
			if bs.gid == gid {
				fmt.Fprintln(&opts, header, "Recursive locking:")
				fmt.Fprintf(&opts, "current goroutine %d lock %p\n", gid, b)
				printStack(&opts, stack)
				fmt.Fprintln(&opts, "Previous place where the lock was grabbed (same goroutine)")
				printStack(&opts, bs.stack)
				l.otherLocked(&opts, curMtx)
				_ = opts.Flush()
				opts.PotentialDeadlock()
			}
			continue
		}
		if bs.gid != gid { // We want locks taken in the same goroutine only.
			continue
		}
		if s, ok := l.order[beforeAfter{curMtx, b}]; ok {
			fmt.Fprintln(&opts, header, "Inconsistent locking. saw this ordering in one goroutine:")
			fmt.Fprintln(&opts, "happened before")
			printStack(&opts, s.before)
			fmt.Fprintln(&opts, "happened after")
			printStack(&opts, s.after)
			fmt.Fprintln(&opts, "in another goroutine: happened before")
			printStack(&opts, bs.stack)
			fmt.Fprintln(&opts, "happened after")
			printStack(&opts, stack)
			l.otherLocked(&opts, curMtx)
			fmt.Fprintln(&opts)
			_ = opts.Flush()
			opts.PotentialDeadlock()
		}
		l.order[beforeAfter{b, curMtx}] = ss{bs.stack, stack}
		// Reset the map to keep memory footprint bounded
		if len(l.order) >= opts.MaxMapSize {
			// This gets optimized to calling runtime.mapclear()
			for k := range l.order {
				delete(l.order, k)
			}
		}
	}
}

func (l *lockOrder) postUnlock(p interface{}) {
	l.mu.Lock()
	delete(l.cur, p)
	l.mu.Unlock()
}

func (l *lockOrder) otherLocked(opts *Options, curMtx interface{}) {
	empty := true
	for k := range l.cur {
		if k == curMtx {
			continue
		}
		empty = false
	}
	if empty {
		return
	}
	fmt.Fprintln(opts, "Other goroutines holding locks:")
	for k, pp := range l.cur {
		if k == curMtx {
			continue
		}
		fmt.Fprintf(opts, "goroutine %v lock %p\n", pp.gid, k)
		printStack(opts, pp.stack)
	}
	fmt.Fprintln(opts)
}

const header = "POTENTIAL DEADLOCK:"
