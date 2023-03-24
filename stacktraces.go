package deadlock

import (
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync/atomic"
)

func callers(skip int) []uintptr {
	s := make([]uintptr, 50) // Most relevant context seem to appear near the top of the stack.
	return s[:runtime.Callers(2+skip, s)]
}

func printStack(w io.Writer, stack []uintptr) {
	for _, pc := range stack {
		if f := runtime.FuncForPC(pc); f != nil {
			var pkg string
			name := f.Name()
			if pos := strings.LastIndexByte(name, '/'); pos >= 0 {
				name = name[pos+1:]
			}
			if pos := strings.IndexByte(name, '.'); pos >= 0 {
				pkg = name[:pos]
				name = name[pos+1:]
				if (pkg == "runtime" && name == "goexit") || (pkg == "testing" && name == "tRunner") {
					break
				}
			}
			file, line := f.FileLine(pc)
			fmt.Fprintf(w, "  %s()\n", f.Name())
			fmt.Fprintf(w, "      %s:%d +0x%x\n", file, line-1, pc-f.Entry())
		}
	}
	fmt.Fprintln(w)
}

var stackBufSize = int64(1024)

// Stacktraces for all goroutines.
func stacks() []byte {
	for {
		bufSize := atomic.LoadInt64(&stackBufSize)
		buf := make([]byte, bufSize)
		if n := runtime.Stack(buf, true); n < len(buf) {
			return buf[:n]
		}
		atomic.StoreInt64(&stackBufSize, bufSize*2)
	}
}
