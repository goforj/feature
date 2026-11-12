package examples_test

import (
	"bytes"
	"context"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPrograms executes every canonical program and requires its declared output.
func TestPrograms(t *testing.T) {
	files, err := filepath.Glob("*/main.go")
	if err != nil || len(files) == 0 {
		t.Fatal("no examples", err)
	}
	for _, filename := range files {
		t.Run(filepath.Dir(filename), func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), filename, nil, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			var expected string
			outputs := 0
			for _, group := range file.Comments {
				if output, ok := strings.CutPrefix(group.Text(), "Output:\n"); ok {
					expected = output
					outputs++
				}
			}
			if outputs != 1 {
				t.Fatal("expected exactly one Output comment")
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			binary := filepath.Join(t.TempDir(), "example")
			args := []string{"build", "-o", binary}
			if exampleRace {
				args = append(args, "-race")
			}
			coverage := os.Getenv("FEATURE_EXAMPLE_COVERAGE_DIR")
			if coverage != "" {
				args = append(args, "-cover", "-coverpkg=github.com/goforj/feature/...")
			}
			args = append(args, "./"+filepath.Dir(filename))
			cmd := exec.CommandContext(ctx, "go", args...)
			cmd.Env = append(os.Environ(), "GOWORK=off")
			if coverage != "" {
				cmd.Env = append(cmd.Env, "GOCOVERDIR="+coverage)
			}
			var stderr bytes.Buffer
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("build: %v\n%s", err, output)
			}
			run := exec.CommandContext(ctx, binary)
			run.Env = cmd.Env
			run.Stderr = &stderr
			actual, err := run.Output()
			if err != nil {
				t.Fatalf("run: %v\n%s", err, stderr.String())
			}
			if string(actual) != expected {
				t.Fatalf("got:\n%swant:\n%s", actual, expected)
			}
		})
	}
}
