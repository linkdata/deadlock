# Preface

Based on https://github.com/sasha-s/go-deadlock.

Changes from that package:
* Uses build tags to eliminate all overhead when not in use
* Only supports Go 1.16+
* Simplifies the code and improves code coverage
* Tests now pass race checker
* This package drops the dummy implementations for types other than Mutex and RWMutex, just use sync for those
* Adds `deadlock.Enabled` constant
* Diagnostic output matches `-race` style

## Installation

```sh
go get github.com/linkdata/deadlock
```

## Usage

The package enables itself when either the 'deadlock' or 'race' build tag is set, and the
'nodeadlock' build tag is not set. The easiest way is to simply use `deadlock.(RW)Mutex` and
run or test your code with the race detector.

```go
import "github.com/linkdata/deadlock"

// Use normally, it works exactly like sync.Mutex does.
var mu deadlock.Mutex
mu.Lock()
defer mu.Unlock()

// Or, using a RWMutex, same procedure
var rw deadlock.RWMutex
rw.RLock()
defer rw.RUnlock()
```

```sh
go run -race ./...
```

### Deadlocks

One of the most common sources of deadlocks is inconsistent lock ordering:
say, you have two mutexes A and B, and in some goroutines you have
```go
A.Lock() // defer A.Unlock() or similar.
...
B.Lock() // defer B.Unlock() or similar.
```
And in another goroutine the order of locks is reversed:
```go
B.Lock() // defer B.Unlock() or similar.
...
A.Lock() // defer A.Unlock() or similar.
```

Another common sources of deadlocks is duplicate take a lock in a goroutine:
```go
A.RLock() // or A.Lock()
...
A.Lock() // or A.RLock()
```

This does not guarantee a deadlock (maybe the goroutines above can never be running at the same time), but it usually a design flaw at least.

deadlock can detect such cases (unless you cross goroutine boundary - say lock A, then spawn a goroutine, block until it is signals, and lock B inside of the goroutine), even if the deadlock itself happens very infrequently and is painful to reproduce!

Each time deadlock sees a lock attempt for lock B, it records the order A before B, for each lock that is currently being held in the same goroutine, and it prints (and exits the program by default) when it sees the locking order being violated.

In addition, if it sees that we are waiting on a lock for a long time (opts.DeadlockTimeout, 30 seconds by default), it reports a potential deadlock, also printing the stacktrace for a goroutine that is currently holding the lock we are desperately trying to grab.

## Sample output

#### Inconsistent lock ordering:

```
POTENTIAL DEADLOCK: Inconsistent locking. saw this ordering in one goroutine:
happened before
  github.com/linkdata/deadlock.TestLockOrder.func2()
      /home/user/src/deadlock/deadlock_test.go:80 +0x111
  github.com/linkdata/deadlock.(*DeadlockRWMutex).Lock()
      /home/user/src/deadlock/deadlock.go:51 +0x8e

happened after
  github.com/linkdata/deadlock.TestLockOrder.func2()
      /home/user/src/deadlock/deadlock_test.go:81 +0x191
  github.com/linkdata/deadlock.(*DeadlockMutex).Lock()
      /home/user/src/deadlock/deadlock.go:22 +0x112

in another goroutine: happened before
  github.com/linkdata/deadlock.TestLockOrder.func3()
      /home/user/src/deadlock/deadlock_test.go:90 +0x10b
  github.com/linkdata/deadlock.(*DeadlockMutex).Lock()
      /home/user/src/deadlock/deadlock.go:22 +0x8e

happened after
  github.com/linkdata/deadlock.TestLockOrder.func3()
      /home/user/src/deadlock/deadlock_test.go:91 +0x191
  github.com/linkdata/deadlock.(*DeadlockRWMutex).RLock()
      /home/user/src/deadlock/deadlock.go:70 +0x10c
```

#### Waiting for a lock for a long time:

```
POTENTIAL DEADLOCK:
Previous place where the lock was grabbed
goroutine 375 lock 0xc0003b21a8
  github.com/linkdata/deadlock.TestHardDeadlock()
      /home/user/src/deadlock/deadlock_test.go:114 +0x165
  github.com/linkdata/deadlock.(*DeadlockMutex).Lock()
      /home/user/src/deadlock/deadlock.go:22 +0xe7

Have been trying to lock it again for more than 20ms
goroutine 377 lock 0xc0003b21a8
  github.com/linkdata/deadlock.TestHardDeadlock.func2()
      /home/user/src/deadlock/deadlock_test.go:118 +0x114
  github.com/linkdata/deadlock.(*DeadlockMutex).Lock()
      /home/user/src/deadlock/deadlock.go:22 +0x93

Here is what goroutine 375 doing now
goroutine 375 [select]:
github.com/linkdata/deadlock.TestHardDeadlock(0xc0003a2d00)
        /home/user/src/deadlock/deadlock_test.go:121 +0x2e5
testing.tRunner(0xc0003a2d00, 0x61a618)
        /usr/local/go/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
        /usr/local/go/src/testing/testing.go:1629 +0x806
All current goroutines:
goroutine 378 [running]:
github.com/linkdata/deadlock.stacks()
        /home/user/src/deadlock/stacktraces.go:46 +0xdf
github.com/linkdata/deadlock.lock.func2()
        /home/user/src/deadlock/deadlock.go:125 +0x5d0
created by github.com/linkdata/deadlock.lock
        /home/user/src/deadlock/deadlock.go:105 +0x34f
```

## Configuring

Have a look at [Opts](https://pkg.go.dev/github.com/linkdata/deadlock#pkg-variables).

* `Opts.DeadlockTimeout`: blocking on mutex for longer than DeadlockTimeout is considered a deadlock. ignored if negative
* `Opts.OnPotentialDeadlock`: callback for then deadlock is detected
* `Opts.MaxMapSize`: size of happens before // happens after table, can also disable order based deadlock detection
* `Opts.PrintAllCurrentGoroutines`:  dump stacktraces of all goroutines when inconsistent locking is detected, verbose
* `Opts.LogBuf`: where to write deadlock info/stacktraces
