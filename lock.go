package deadlock

import (
	"bytes"
	"fmt"
	"time"

	"github.com/petermattis/goid"
)

const header = "POTENTIAL DEADLOCK:"

func lock(tryLockFn func() bool, lockFn func(), curMtx interface{}) {
	var opts Options
	Opts.ReadLocked(func() { opts = Opts })
	gid := goid.Get()
	curStack := callers(2)

	if opts.MaxMapSize > 0 {
		lo.preLock(&opts, gid, curStack, curMtx)
	}

	if tryLockFn == nil || !tryLockFn() {
		if opts.DeadlockTimeout > 0 {
			ch := make(chan struct{})
			defer close(ch)
			go func() {
				for {
					t := time.NewTimer(opts.DeadlockTimeout)
					defer t.Stop() // This runs after the closure finishes, but it's OK.
					select {
					case <-t.C:
						fmt.Fprintln(&opts, header)
						fmt.Fprintf(&opts, "goroutine %v have been trying to lock %p for more than %v:\n",
							gid, curMtx, opts.DeadlockTimeout)
						printStack(&opts, curStack)

						curStacks := stacks()

						func() {
							lo.mu.Lock()
							defer lo.mu.Unlock()
							if prev, ok := lo.cur[curMtx]; ok {
								fmt.Fprintf(&opts, "goroutine %v previously locked it from:\n", prev.gid)
								printStack(&opts, prev.stack)
								goroutineStackList := bytes.Split(curStacks, []byte("\n\n"))
								for _, goroutineStack := range goroutineStackList {
									if goid.ExtractGID(goroutineStack) == prev.gid {
										fmt.Fprintf(&opts, "goroutine %v current stack:\n", prev.gid)
										_, _ = opts.Write(goroutineStack)
										fmt.Fprintln(&opts)
									}
								}
							}
							lo.otherLocked(&opts, curMtx)
						}()

						if opts.PrintAllCurrentGoroutines {
							fmt.Fprintln(&opts, "All current goroutines:")
							_, _ = opts.Write(curStacks)
						}

						fmt.Fprintln(&opts)
						_ = opts.Flush()
						opts.PotentialDeadlock()
						<-ch
						return
					case <-ch:
						return
					}
				}
			}()
		}
		lockFn()
	}

	lo.postLock(gid, curStack, curMtx)
}
