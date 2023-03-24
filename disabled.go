//go:build !deadlock && !race
// +build !deadlock,!race

package deadlock

import "sync"

// Mutex is sync.Mutex wrapper
type Mutex struct{ sync.Mutex }

// RWMutex is sync.RWMutex wrapper
type RWMutex struct{ sync.RWMutex }

// Enabled is true if deadlock checking is enabled
const Enabled = false
