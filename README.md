# Preface

Based on https://github.com/sasha-s/go-deadlock.

Changes from that package:
* Supports Go 1.16+
* Uses build tags to eliminate all overhead when not in use

## Why
Deadlocks happen and are painful to debug.

## What
This package provides (RW)Mutex drop-in replacements for sync.(RW)Mutex.

## Installation
```sh
go get github.com/linkdata/deadlock/...
```

## Usage
```go
import "github.com/linkdata/deadlock"
var mu deadlock.Mutex
// Use normally, it works exactly like sync.Mutex does.
mu.Lock()

defer mu.Unlock()
// Or
var rw deadlock.RWMutex
rw.RLock()
defer rw.RUnlock()
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
```
A.Rlock() or lock()

A.lock() or A.RLock()
```

This does not guarantee a deadlock (maybe the goroutines above can never be running at the same time), but it usually a design flaw at least.

go-deadlock can detect such cases (unless you cross goroutine boundary - say lock A, then spawn a goroutine, block until it is singals, and lock B inside of the goroutine), even if the deadlock itself happens very infrequently and is painful to reproduce!

Each time go-deadlock sees a lock attempt for lock B, it records the order A before B, for each lock that is currently being held in the same goroutine, and it prints (and exits the program by default) when it sees the locking order being violated.

In addition, if it sees that we are waiting on a lock for a long time (opts.DeadlockTimeout, 30 seconds by default), it reports a potential deadlock, also printing the stacktrace for a goroutine that is currently holding the lock we are desperately trying to grab.


## Sample output
#### Inconsistent lock ordering:
```
POTENTIAL DEADLOCK: Inconsistent locking. saw this ordering in one goroutine:
happened before
inmem.go:623 bttest.(*server).ReadModifyWriteRow { r.mu.Lock() } <<<<<
inmem_test.go:118 bttest.TestConcurrentMutationsReadModifyAndGC.func4 { _, _ = s.ReadModifyWriteRow(ctx, rmw()) }

happened after
inmem.go:629 bttest.(*server).ReadModifyWriteRow { tbl.mu.RLock() } <<<<<
inmem_test.go:118 bttest.TestConcurrentMutationsReadModifyAndGC.func4 { _, _ = s.ReadModifyWriteRow(ctx, rmw()) }

in another goroutine: happened before
inmem.go:799 bttest.(*table).gc { t.mu.RLock() } <<<<<
inmem_test.go:125 bttest.TestConcurrentMutationsReadModifyAndGC.func5 { tbl.gc() }

happend after
inmem.go:814 bttest.(*table).gc { r.mu.Lock() } <<<<<
inmem_test.go:125 bttest.TestConcurrentMutationsReadModifyAndGC.func5 { tbl.gc() }
```

#### Waiting for a lock for a long time:

```
POTENTIAL DEADLOCK:
Previous place where the lock was grabbed
goroutine 240 lock 0xc820160440
inmem.go:799 bttest.(*table).gc { t.mu.RLock() } <<<<<
inmem_test.go:125 bttest.TestConcurrentMutationsReadModifyAndGC.func5 { tbl.gc() }

Have been trying to lock it again for more than 40ms
goroutine 68 lock 0xc820160440
inmem.go:785 bttest.(*table).mutableRow { t.mu.Lock() } <<<<<
inmem.go:428 bttest.(*server).MutateRow { r := tbl.mutableRow(string(req.RowKey)) }
inmem_test.go:111 bttest.TestConcurrentMutationsReadModifyAndGC.func3 { s.MutateRow(ctx, req) }


Here is what goroutine 240 doing now
goroutine 240 [select]:
github.com/linkdata/deadlock.lock(0xc82028ca10, 0x5189e0, 0xc82013a9b0)
        /Users/sasha/go/src/github.com/linkdata/deadlock/deadlock.go:163 +0x1640
github.com/linkdata/deadlock.(*Mutex).Lock(0xc82013a9b0)
        /Users/sasha/go/src/github.com/linkdata/deadlock/deadlock.go:54 +0x86
google.golang.org/cloud/bigtable/bttest.(*table).gc(0xc820160440)
        /Users/sasha/go/src/google.golang.org/cloud/bigtable/bttest/inmem.go:814 +0x28d
google.golang.org/cloud/bigtable/bttest.TestConcurrentMutationsReadModifyAndGC.func5(0xc82015c760, 0xc820160440)      /Users/sasha/go/src/google.golang.org/cloud/bigtable/bttest/inmem_test.go:125 +0x48
created by google.golang.org/cloud/bigtable/bttest.TestConcurrentMutationsReadModifyAndGC
        /Users/sasha/go/src/google.golang.org/cloud/bigtable/bttest/inmem_test.go:126 +0xb6f
```

## Configuring

Have a look at [Opts](https://pkg.go.dev/github.com/linkdata/deadlock#pkg-variables).

* `Opts.Disable`: disables deadlock detection altogether
* `Opts.DisableLockOrderDetection`: disables lock order based deadlock detection.
* `Opts.DeadlockTimeout`: blocking on mutex for longer than DeadlockTimeout is considered a deadlock. ignored if negative
* `Opts.OnPotentialDeadlock`: callback for then deadlock is detected
* `Opts.MaxMapSize`: size of happens before // happens after table
* `Opts.PrintAllCurrentGoroutines`:  dump stacktraces of all goroutines when inconsistent locking is detected, verbose
* `Opts.LogBuf`: where to write deadlock info/stacktraces

	
