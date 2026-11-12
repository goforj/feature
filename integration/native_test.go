//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/goforj/feature"
)

// TestFileRefreshLifecycle exercises real atomic replacements while requests retain snapshots.
func TestFileRefreshLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flags.json")
	publish := func(contents string) {
		t.Helper()
		if err := os.WriteFile(path+".next", []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(path+".next", path); err != nil {
			t.Fatal(err)
		}
	}
	publish(`{"schema":1,"flags":{"checkout":{"default":false},"billing":{"default":false}}}`)
	client, err := feature.Open(context.Background(), feature.NewFileSource(path))
	if err != nil {
		t.Fatal(err)
	}
	old := client.Snapshot()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- client.Run(ctx, 10*time.Millisecond) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != context.Canceled {
				t.Error(err)
			}
		case <-time.After(2 * time.Second):
			t.Error("refresh worker did not stop")
		}
	})
	var readers sync.WaitGroup
	readCtx, stopReaders := context.WithCancel(context.Background())
	for i := 0; i < 8; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for readCtx.Err() == nil {
				view := client.Snapshot()
				a := view.Evaluate(ctx, "checkout", false, feature.Subject{})
				b := view.Evaluate(ctx, "billing", false, feature.Subject{})
				if a.Err != nil || b.Err != nil || a.Value != b.Value || a.Revision != b.Revision {
					t.Error("request observed a partial revision")
					return
				}
			}
		}()
	}
	t.Cleanup(func() { stopReaders(); readers.Wait() })
	publish(`{"schema":1,"flags":{"checkout":{"default":true},"billing":{"default":true}}}`)
	eventually(t, func() bool { return client.Enabled(ctx, "checkout", false, feature.Subject{}) })
	valid := client.Status().Revision
	if old.Enabled(ctx, "checkout", false, feature.Subject{}) || old.Evaluate(ctx, "checkout", false, feature.Subject{}).Revision == valid {
		t.Fatal("captured snapshot changed")
	}
	publish(`{"schema":1,"flags":{"checkout":{"default":false},"billing":{"default":`)
	eventually(t, func() bool { return client.Status().LastError == feature.ErrInvalidDocument })
	if client.Status().Revision != valid || !client.Enabled(ctx, "checkout", false, feature.Subject{}) {
		t.Fatal("invalid document replaced the last valid revision")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return client.Status().LastError == feature.ErrSource })
	if client.Status().Revision != valid {
		t.Fatal("missing file discarded the last valid revision")
	}
	publish(`{"schema":1,"flags":{"checkout":{"default":false},"billing":{"default":false}}}`)
	eventually(t, func() bool {
		return client.Status().LastError == nil && !client.Enabled(ctx, "checkout", true, feature.Subject{})
	})
}
