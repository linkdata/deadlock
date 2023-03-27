package deadlock

import (
	"fmt"
	"sync"
)

type lockOrder struct {
	mu    sync.Mutex                          // protects following
	cur   map[interface{}]stackGID            // stacktraces + gids for the locks currently taken.
	order map[beforeAfterMtx]beforeAfterStack // expected order of locks.
}

type stackGID struct {
	stack []uintptr
	gid   int64
}

type beforeAfterMtx struct {
	beforeMtx interface{}
	afterMtx  interface{}
}

type beforeAfterStack struct {
	beforeStack []uintptr
	afterStack  []uintptr
}

var lo = newLockOrder()

func newLockOrder() *lockOrder {
	return &lockOrder{
		cur:   map[interface{}]stackGID{},
		order: map[beforeAfterMtx]beforeAfterStack{},
	}
}

func (l *lockOrder) postLock(gid int64, curStack []uintptr, curMtx interface{}) {
	l.mu.Lock()
	l.cur[curMtx] = stackGID{curStack, gid}
	l.mu.Unlock()
}

func (l *lockOrder) preLock(opts *Options, gid int64, curStack []uintptr, curMtx interface{}) {
	if opts.MaxMapSize < 1 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for otherMtx, otherStackGID := range l.cur {
		if otherMtx == curMtx {
			if otherStackGID.gid == gid {
				fmt.Fprintln(opts, header, "Recursive locking:")
				fmt.Fprintf(opts, "goroutine %d lock %p:\n", gid, otherMtx)
				printStack(opts, curStack)
				fmt.Fprintln(opts, "same goroutine previously locked it from:")
				printStack(opts, otherStackGID.stack)
				l.otherLocked(opts, curMtx)
				_ = opts.Flush()
				opts.PotentialDeadlock()
			}
			continue
		}
		if otherStackGID.gid != gid { // We want locks taken in the same goroutine only.
			continue
		}
		if otherStacks, ok := l.order[beforeAfterMtx{curMtx, otherMtx}]; ok {
			fmt.Fprintln(opts, header, "Inconsistent locking:")
			fmt.Fprintln(opts, "in one goroutine: happened before")
			printStack(opts, otherStacks.beforeStack)
			fmt.Fprintln(opts, "happened after")
			printStack(opts, otherStacks.afterStack)

			fmt.Fprintln(opts, "in another goroutine: happened before")
			printStack(opts, otherStackGID.stack)
			fmt.Fprintln(opts, "happened after")
			printStack(opts, curStack)
			l.otherLocked(opts, curMtx)
			fmt.Fprintln(opts)
			_ = opts.Flush()
			opts.PotentialDeadlock()
		}
		l.order[beforeAfterMtx{otherMtx, curMtx}] = beforeAfterStack{otherStackGID.stack, curStack}
		// Reset the map to keep memory footprint bounded
		if len(l.order) >= opts.MaxMapSize {
			// This gets optimized to calling runtime.mapclear()
			for k := range l.order {
				delete(l.order, k)
			}
		}
	}
}

func (l *lockOrder) postUnlock(curMtx interface{}) {
	l.mu.Lock()
	delete(l.cur, curMtx)
	l.mu.Unlock()
}

func (l *lockOrder) otherLocked(opts *Options, curMtx interface{}) {
	printedHeader := false
	for otherMtx, otherStackGID := range l.cur {
		if otherMtx != curMtx {
			if !printedHeader {
				printedHeader = true
				fmt.Fprintln(opts, "Other goroutines holding locks:")
			}
			fmt.Fprintf(opts, "goroutine %v lock %p\n", otherStackGID.gid, otherMtx)
			printStack(opts, otherStackGID.stack)
		}
	}
	if printedHeader {
		fmt.Fprintln(opts)
	}
}
