package internal

import (
	"sync"
	"testing"

	"github.com/linkdata/deadlock"
	// deadlock "github.com/sasha-s/go-deadlock"
)

// To benchmark CPU and allocations:
//  go test -tags deadlock -run=^$ -benchmem -bench ^Benchmark ./...
// To benchmark detailed memory usage:
//  go test -tags deadlock -run=^$ -benchmem -memprofilerate=1 -memprofile mem.out -bench ^Benchmark .
//  go tool pprof mem.out

func unlock(l sync.Locker) {
	l.Unlock()
}

func BenchmarkLockSingle(b *testing.B) {
	var mu deadlock.Mutex
	for i := 0; i < b.N; i++ {
		mu.Lock()
		unlock(&mu)
	}
}

func BenchmarkLockParallel(b *testing.B) {
	var mu deadlock.Mutex
	b.RunParallel(
		func(p *testing.PB) {
			for p.Next() {
				mu.Lock()
				unlock(&mu)
			}
		})
}
