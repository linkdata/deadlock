package deadlock

import (
	"testing"
)

func Test_testGoid(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic")
		}
	}()
	goid := getGoidFallback()
	testGoid(goid + 1)
}
