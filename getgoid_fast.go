//go:build !slowgoid
// +build !slowgoid

package deadlock

import (
	"github.com/petermattis/goid"
)

func getGoid() int64 {
	return goid.Get()
}

func goidMatches(slowId int64) bool {
	return goid.Get() == slowId
}
