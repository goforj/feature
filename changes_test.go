package feature

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

// TestChangeLifecycle verifies successful publication, failure transitions, recovery, and listener ownership.
func TestChangeLifecycle(t *testing.T) {
	ctx := context.Background()
	document := sampleDocument()
	fail := false
	var notifications []Change
	clock := time.Unix(100, 0)
	source := SourceFunc(func(context.Context) (Document, error) {
		if fail {
			return Document{}, errors.New("private source")
		}
		return document, nil
	})
	engine, err := Open(ctx, source, WithClock(func() time.Time { return clock }), WithNotifier(NotifierFunc(func(_ context.Context, change Change) error {
		notifications = append(notifications, change)
		return errors.New("private webhook")
	})))
	if err != nil {
		t.Fatal(err)
	}
	if engine.Snapshot().at != clock || engine.Status().LastNotifyError != ErrUnavailable || len(notifications) != 1 {
		t.Fatal(engine.Status(), notifications)
	}
	subscription, err := engine.Subscribe()
	if err != nil {
		t.Fatal(err)
	}
	initial := <-subscription.Changes
	if initial.Revision != engine.Status().Revision || initial.Err != nil {
		t.Fatal(initial)
	}
	if err := engine.Refresh(ctx); err != nil || len(notifications) != 1 {
		t.Fatal("unchanged refresh notified", err)
	}
	document = Document{Schema: 1, Flags: map[string]Flag{"replacement": {Default: true}}}
	if err := engine.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	change := <-subscription.Changes
	if change.PreviousRevision != initial.Revision || !reflect.DeepEqual(change.Keys, []string{"checkout-v2", "replacement"}) {
		t.Fatal(change)
	}
	fail = true
	if err := engine.Refresh(ctx); err != ErrSource {
		t.Fatal(err)
	}
	if change := <-subscription.Changes; change.Err != ErrSource || change.Revision != engine.Status().Revision {
		t.Fatal(change)
	}
	before := len(notifications)
	if err := engine.Refresh(ctx); err != ErrSource || len(notifications) != before {
		t.Fatal("repeated failure notified", err)
	}
	fail = false
	if err := engine.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if change := <-subscription.Changes; change.Err != nil {
		t.Fatal(change)
	}
	if err := subscription.Close(); err != nil {
		t.Fatal(err)
	}
	if err := subscription.Close(); err != nil {
		t.Fatal(err)
	}
	if _, open := <-subscription.Changes; open {
		t.Fatal("closed subscription retained")
	}
	if err := engine.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
}

// TestBoundedSubscriptions verifies coalescing, detached payloads, limits, and concurrent cleanup.
func TestBoundedSubscriptions(t *testing.T) {
	ctx := context.Background()
	engine, source := openMemory(t)
	var subscriptions []*Subscription
	for i := 0; i < MaxSubscriptions; i++ {
		subscription, err := engine.Subscribe()
		if err != nil {
			t.Fatal(err)
		}
		subscriptions = append(subscriptions, subscription)
	}
	if _, err := engine.Subscribe(); err != ErrUnavailable {
		t.Fatal(err)
	}
	if err := source.Replace(Document{Schema: 1, Flags: map[string]Flag{"next": {}}}); err != nil {
		t.Fatal(err)
	}
	if err := engine.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	for _, subscription := range subscriptions {
		change := <-subscription.Changes
		if change.Keys != nil || change.Revision != engine.Status().Revision {
			t.Fatal("slow listener did not receive coalesced latest revision", change)
		}
	}
	if err := source.Replace(Document{Schema: 1, Flags: map[string]Flag{"new": {}}}); err != nil {
		t.Fatal(err)
	}
	if err := engine.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	first := <-subscriptions[0].Changes
	first.Keys[0] = "mutated"
	second := <-subscriptions[1].Changes
	if second.Keys[0] == "mutated" {
		t.Fatal("listeners shared mutable keys")
	}
	var group sync.WaitGroup
	for _, subscription := range subscriptions {
		group.Add(1)
		go func() { defer group.Done(); _ = subscription.Close() }()
	}
	group.Wait()
	if len(engine.subscribers) != 0 {
		t.Fatal("listeners retained")
	}
}
