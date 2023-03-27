package deadlock

import (
	"bufio"
	"os"
	"testing"
)

func TestOptions_Write_LogBufIsNil(t *testing.T) {
	opts := Options{}
	if n, err := opts.Write([]byte("foo")); err == nil {
		if n != 0 {
			t.Fail()
		}
	} else {
		t.Error(err)
	}
	if err := opts.Flush(); err != nil {
		t.Error(err)
	}
}

func TestOptions_Write_LogBufIsBufio(t *testing.T) {
	var fooText = []byte("foo")
	opts := Options{
		LogBuf: bufio.NewWriter(os.Stderr),
	}
	if n, err := opts.Write([]byte("foo")); err == nil {
		if n != len(fooText) {
			t.Fail()
		}
	} else {
		t.Error(err)
	}
	if err := opts.Flush(); err != nil {
		t.Error(err)
	}
}
