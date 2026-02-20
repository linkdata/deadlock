//go:build slowgoid
// +build slowgoid

package deadlock

func getGoid() int64 {
	return getGoidFallback()
}

func goidMatches(slowId int64) bool {
	return getGoidFallback() == slowId
}
