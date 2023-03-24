//go:build !deadlock
// +build !deadlock

package deadlock

import "sync"

// Mutex is sync.Mutex wrapper
type Mutex struct {
	sync.Mutex
}

// RWMutex is sync.RWMutex wrapper
type RWMutex struct {
	sync.RWMutex
}
