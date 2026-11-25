//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package feature

import "os"

// openSourceFile preserves acquisition on platforms without Unix nonblocking opens.
// Go's JavaScript and WASI filesystem layers do not honor O_NONBLOCK, and Plan 9 defines it as zero.
func openSourceFile(path string) (*os.File, error) {
	return os.Open(path)
}
