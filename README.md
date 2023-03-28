[![build](https://github.com/linkdata/deadlock/actions/workflows/go.yml/badge.svg)](https://github.com/linkdata/deadlock/actions/workflows/go.yml)
[![coverage](https://coveralls.io/repos/github/linkdata/deadlock/badge.svg?branch=main)](https://coveralls.io/github/linkdata/deadlock?branch=main)
[![goreport](https://goreportcard.com/badge/github.com/linkdata/deadlock)](https://goreportcard.com/report/github.com/linkdata/deadlock)

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

var mu deadlock.Mutex
mu.Lock()
defer mu.Unlock()

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
POTENTIAL DEADLOCK: Inconsistent locking:
in one goroutine: happened before
  github.com/linkdata/deadlock.(*DeadlockRWMutex).Lock()
      /home/user/src/deadlock/deadlock.go:55 +0xa8
  github.com/linkdata/deadlock.TestLockOrder.func2()
      /home/user/src/deadlock/deadlock_test.go:120 +0x34

happened after
  github.com/linkdata/deadlock.(*DeadlockMutex).Lock()
      /home/user/src/deadlock/deadlock.go:26 +0x11a
  github.com/linkdata/deadlock.TestLockOrder.func2()
      /home/user/src/deadlock/deadlock_test.go:121 +0xa9

in another goroutine: happened before
  github.com/linkdata/deadlock.(*DeadlockMutex).Lock()
      /home/user/src/deadlock/deadlock.go:26 +0xa5
  github.com/linkdata/deadlock.TestLockOrder.func3()
      /home/user/src/deadlock/deadlock_test.go:129 +0x34

happened after
  github.com/linkdata/deadlock.(*DeadlockRWMutex).RLock()
      /home/user/src/deadlock/deadlock.go:74 +0x11a
  github.com/linkdata/deadlock.TestLockOrder.func3()
      /home/user/src/deadlock/deadlock_test.go:130 +0xa6
```

#### Waiting for a lock for a long time:

```
POTENTIAL DEADLOCK:
goroutine 624 have been trying to lock 0xc0009a20d8 for more than 20ms:
  github.com/linkdata/deadlock.(*DeadlockMutex).Lock()
      /home/user/src/deadlock/deadlock.go:26 +0x113
  github.com/linkdata/deadlock.TestHardDeadlock.func2()
      /home/user/src/deadlock/deadlock_test.go:154 +0x92

goroutine 622 previously locked it from:
  github.com/linkdata/deadlock.(*DeadlockMutex).Lock()
      /home/user/src/deadlock/deadlock.go:26 +0x164
  github.com/linkdata/deadlock.TestHardDeadlock()
      /home/user/src/deadlock/deadlock_test.go:150 +0xe6

goroutine 622 current stack:
goroutine 622 [sleep]:
time.Sleep(0xf4240)
        /usr/local/go/src/runtime/time.go:195 +0x135
github.com/linkdata/deadlock.spinWait(0xc000988340, 0x0?, 0x1)
        /home/user/src/deadlock/deadlock_test.go:25 +0x3e
github.com/linkdata/deadlock.TestHardDeadlock(0xc000988340)
        /home/user/src/deadlock/deadlock_test.go:157 +0x265
testing.tRunner(0xc000988340, 0x6187e8)
        /usr/local/go/src/testing/testing.go:1576 +0x217
created by testing.(*T).Run
        /usr/local/go/src/testing/testing.go:1629 +0x806
```

## Configuring

Have a look at [Opts](https://pkg.go.dev/github.com/linkdata/deadlock#pkg-variables).

* `Opts.DeadlockTimeout`: blocking on mutex for longer than DeadlockTimeout is considered a deadlock. ignored if negative
* `Opts.OnPotentialDeadlock`: callback for then deadlock is detected
* `Opts.MaxMapSize`: size of happens before // happens after table, can also disable order based deadlock detection
* `Opts.PrintAllCurrentGoroutines`:  dump stacktraces of all goroutines when inconsistent locking is detected, verbose
* `Opts.LogBuf`: where to write deadlock info/stacktraces
