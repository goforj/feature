package feature

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// sourceFunc supplies controlled source outcomes without an optional dependency.
type sourceFunc func(context.Context) (Document, error)

// Load delegates to the test's source behavior.
func (f sourceFunc) Load(ctx context.Context) (Document, error) { return f(ctx) }

// openMemory constructs a complete client or fails the owning test immediately.
func openMemory(t *testing.T) (*Client, *MemorySource) {
	t.Helper()
	s, err := NewMemorySource(sampleDocument())
	if err != nil {
		t.Fatal(err)
	}
	c, err := Open(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	return c, s
}

// TestClientRefreshAndViews checks atomic publication, detached status, and old-view consistency.
func TestClientRefreshAndViews(t *testing.T) {
	ctx := context.Background()
	c, s := openMemory(t)
	view := c.Snapshot()
	before := c.Status()
	if before.Revision == "" || before.LastSuccess.IsZero() || before.LastError != nil {
		t.Fatal(before)
	}
	if err := c.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if c.Status().Revision != before.Revision {
		t.Fatal("unchanged revision changed")
	}
	if !c.Enabled(ctx, "checkout-v2", false, Subject{Key: "tenant:internal"}) {
		t.Fatal("expected enabled")
	}
	if !c.Evaluate(ctx, "missing", true, Subject{}).Value {
		t.Fatal("lost fallback")
	}
	if c.Status().Evaluations != 2 || c.Status().Fallbacks != 1 {
		t.Fatal(c.Status())
	}
	if err := s.Replace(Document{Schema: 1, Flags: map[string]Flag{"checkout-v2": {Disabled: true}}}); err != nil {
		t.Fatal(err)
	}
	if !c.Enabled(ctx, "checkout-v2", false, Subject{Key: "tenant:internal"}) {
		t.Fatal("source replacement skipped refresh")
	}
	if err := c.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if c.Enabled(ctx, "checkout-v2", true, Subject{}) || c.Status().Revision == before.Revision {
		t.Fatal("refresh was not published")
	}
	if !view.Enabled(ctx, "checkout-v2", false, Subject{Key: "tenant:internal"}) {
		t.Fatal("old view changed")
	}
	detached := c.Status()
	detached.Revision = "changed"
	if c.Status().Revision == detached.Revision {
		t.Fatal("status borrowed data")
	}
	if err := s.Replace(Document{Schema: 1, Flags: map[string]Flag{}}); err != nil {
		t.Fatal(err)
	}
	if err := c.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if d := c.Evaluate(ctx, "checkout-v2", true, Subject{}); d.Reason != ReasonMissing || !d.Value {
		t.Fatal(d)
	}
}

// TestRefreshFailureClassification proves redaction and retention for every failure stage.
func TestRefreshFailureClassification(t *testing.T) {
	ctx := context.Background()
	var outcome atomic.Int32
	source := sourceFunc(func(ctx context.Context) (Document, error) {
		switch outcome.Load() {
		case 1:
			return Document{}, errors.New("private path and credentials")
		case 2:
			return Document{}, ErrInvalidDocument
		case 3:
			return Document{}, nil
		default:
			return sampleDocument(), nil
		}
	})
	c, err := Open(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	before := c.Status()
	for _, tc := range []struct {
		n    int32
		want error
	}{{1, ErrSource}, {2, ErrInvalidDocument}, {3, ErrInvalidDocument}} {
		outcome.Store(tc.n)
		if err := c.Refresh(ctx); err != tc.want {
			t.Fatal(err)
		}
		status := c.Status()
		if status.Revision != before.Revision || !status.LastSuccess.Equal(before.LastSuccess) || status.LastError != tc.want || status.LastAttempt.IsZero() {
			t.Fatal(status)
		}
	}
	outcome.Store(0)
	if err := c.Refresh(ctx); err != nil || c.Status().LastError != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		n    int32
		want error
	}{{1, ErrSource}, {3, ErrInvalidDocument}} {
		outcome.Store(tc.n)
		if _, err := Open(ctx, source); err != tc.want {
			t.Fatal(err)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := c.Refresh(canceled); err != context.Canceled {
		t.Fatal(err)
	}
	for _, fail := range []bool{false, true} {
		child, cancel := context.WithCancel(ctx)
		source := sourceFunc(func(context.Context) (Document, error) {
			cancel()
			if fail {
				return Document{}, ErrSource
			}
			return sampleDocument(), nil
		})
		if _, err := Open(child, source); err != context.Canceled {
			t.Fatal(err)
		}
	}
}

// TestRefreshWaitingCancellation proves queued callers cancel without starting another source load.
func TestRefreshWaitingCancellation(t *testing.T) {
	ctx := context.Background()
	var block atomic.Bool
	entered := make(chan struct{})
	release := make(chan struct{})
	s := sourceFunc(func(context.Context) (Document, error) {
		if block.Load() {
			close(entered)
			<-release
		}
		return sampleDocument(), nil
	})
	c, err := Open(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	block.Store(true)
	done := make(chan error, 1)
	go func() { done <- c.Refresh(ctx) }()
	<-entered
	queued, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if err := c.Refresh(queued); err != context.DeadlineExceeded {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// TestRunLifecycle proves single-loop ownership, retries, recovery, and restart after cancellation.
func TestRunLifecycle(t *testing.T) {
	ctx := context.Background()
	var mode atomic.Int32
	attempts := make(chan int32, 32)
	s := sourceFunc(func(context.Context) (Document, error) {
		n := mode.Load()
		attempts <- n
		if n == 1 {
			return Document{}, ErrSource
		}
		return sampleDocument(), nil
	})
	c, err := Open(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	<-attempts
	if err := c.Run(ctx, 0); err != ErrInvalidInterval {
		t.Fatal(err)
	}
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	mode.Store(1)
	go func() { done <- c.Run(child, 10*time.Millisecond) }()
	select {
	case <-attempts:
	case <-time.After(2 * time.Second):
		t.Fatal("no polling")
	}
	if err := c.Run(ctx, 10*time.Millisecond); err != ErrAlreadyRunning {
		t.Fatal(err)
	}
	// An explicit refresh waits for the in-flight tick and makes status observation deterministic.
	if err := c.Refresh(ctx); err != ErrSource || c.Status().LastError != ErrSource {
		t.Fatal(err, c.Status())
	}
	mode.Store(0)
	if err := c.Refresh(ctx); err != nil || c.Status().LastError != nil {
		t.Fatal(err)
	}
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatal(err)
	}
	stopped, stop := context.WithCancel(ctx)
	stop()
	if err := c.Run(stopped, 10*time.Millisecond); err != context.Canceled {
		t.Fatal(err)
	}
}

// TestConcurrentPublication checks readers never observe partially compiled replacements.
func TestConcurrentPublication(t *testing.T) {
	ctx := context.Background()
	c, s := openMemory(t)
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				v := c.Snapshot()
				a := v.Evaluate(ctx, "checkout-v2", false, Subject{})
				b := v.Evaluate(ctx, "checkout-v2", false, Subject{})
				if a != b {
					t.Error("view changed")
				}
				_ = c.Status()
			}
		}()
	}
	for i := 0; i < 30; i++ {
		if err := s.Replace(Document{Schema: 1, Flags: map[string]Flag{"checkout-v2": {Default: i%2 == 0}}}); err != nil {
			t.Fatal(err)
		}
		if err := c.Refresh(ctx); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
}
