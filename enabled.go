//go:build deadlock || race
// +build deadlock race

package deadlock

// Mutex is deadlock.DeadlockMutex wrapper
type Mutex struct{ DeadlockMutex }

// RWMutex is deadlock.DeadlockRWMutex wrapper
type RWMutex struct{ DeadlockRWMutex }
