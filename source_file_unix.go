//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package feature

import (
	"os"
	"syscall"
)

// openSourceFile avoids FIFO rendezvous before readFile validates the opened descriptor.
func openSourceFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
