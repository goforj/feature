//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package feature

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestFileSourceFIFO rejects writerless FIFOs after acquisition without a path preflight race.
func TestFileSourceFIFO(t *testing.T) {
	if os.Getenv("FEATURE_TEST_FIFO_CHILD") == "1" {
		path := os.Getenv("FEATURE_TEST_FIFO_PATH")
		file, err := openSourceFile(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if _, err := readFile(context.Background(), file); err != ErrSource {
			t.Fatalf("descriptor validation: got %v", err)
		}
		if _, err := NewFileSource(path).Load(context.Background()); err != ErrSource {
			t.Fatalf("source load: got %v", err)
		}
		return
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "private-source")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "private-link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	for _, sourcePath := range []string{path, link} {
		t.Run(filepath.Base(sourcePath), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestFileSourceFIFO$", "-test.timeout=8s")
			command.Env = append(os.Environ(), "FEATURE_TEST_FIFO_CHILD=1", "FEATURE_TEST_FIFO_PATH="+sourcePath)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("writerless FIFO acquisition failed: %v\n%s", err, output)
			}
		})
	}
}
