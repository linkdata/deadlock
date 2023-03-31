package deadlock

import (
	"github.com/petermattis/goid"
)

func lock(tryLockFn func() bool, lockFn func(), curMtx interface{}) bool {
	var opts Options
	Opts.ReadLocked(func() { opts = Opts })
	gid := goid.Get()
	curStack := callers(2)

	if lockFn != nil && opts.MaxMapSize > 0 {
		lo.preLock(&opts, gid, curStack, curMtx)
	}

	if tryLockFn == nil || !tryLockFn() {
		if lockFn == nil {
			return false
		}
		if opts.DeadlockTimeout > 0 {
			ch := make(chan struct{})
			defer close(ch)
			go lo.timeoutFn(ch, &opts, gid, curStack, curMtx)
		}
		lockFn()
	}

	lo.postLock(gid, curStack, curMtx)
	return true
}
