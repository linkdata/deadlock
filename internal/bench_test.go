package internal

import (
	"sync"
	"testing"

	"github.com/linkdata/deadlock"
	// deadlock "github.com/sasha-s/go-deadlock"
)

// To benchmark CPU:
//  go test -tags deadlock -bench .
// To benchmark memory:
//  go test -tags deadlock -benchmem -memprofilerate=1 -memprofile mem.out -bench .
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

func BenchmarkLockParalell(b *testing.B) {
	var mu deadlock.Mutex
	b.RunParallel(
		func(p *testing.PB) {
			for p.Next() {
				mu.Lock()
				unlock(&mu)
			}
		})
}
