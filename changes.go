package feature

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"
)

// MaxSubscriptions bounds listener ownership within one native client.
const MaxSubscriptions = 64

// Change describes a published revision or a transition into or out of source failure.
// Keys is nil when changes were coalesced and consumers should refresh their complete inventory.
type Change struct {
	Revision         string   `json:"revision"`
	PreviousRevision string   `json:"previous_revision"`
	Keys             []string `json:"keys,omitempty"`
	Err              error    `json:"-"`
}

// Notifier submits native change notifications without owning a refresh loop.
// Failures are recorded separately from successful source publication.
// Notify runs synchronously and must not reenter the owning client's Refresh or Run;
// it may inspect Status. Use a queue when a callback needs to trigger later refreshes.
type Notifier interface {
	Notify(context.Context, Change) error
}

// NotifierFunc adapts a queue, webhook, or application callback to native notifications.
type NotifierFunc func(context.Context, Change) error

// Notify invokes the configured notification destination.
func (f NotifierFunc) Notify(ctx context.Context, change Change) error { return f(ctx, change) }

// ClientOption configures explicit native lifecycle collaborators.
type ClientOption func(*clientConfig)

// clientConfig separates native publication collaborators from portable facade options.
type clientConfig struct {
	clock    func() time.Time
	notifier Notifier
	tracker  Tracker
}

// WithClock supplies evaluation time for scheduled rules and deterministic application tests.
// The clock must be non-nil, local, and safe for concurrent calls.
func WithClock(clock func() time.Time) ClientOption {
	return func(config *clientConfig) { config.clock = clock }
}

// WithNotifier attaches a bounded, application-owned native change destination.
// Callbacks must not reenter this client's Refresh or Run; they may inspect Status.
func WithNotifier(notifier Notifier) ClientOption {
	return func(config *clientConfig) { config.notifier = notifier }
}

// WithEventSink attaches application-owned analytics to the native engine.
func WithEventSink(tracker Tracker) ClientOption {
	return func(config *clientConfig) { config.tracker = tracker }
}

// Subscription owns one bounded native listener; Close unregisters it and closes Changes.
// Slow consumers receive the latest notification with Keys cleared after coalescing.
type Subscription struct {
	Changes <-chan Change
	once    sync.Once
	close   func()
}

// Close synchronizes listener removal with publication and is safe to repeat.
func (s *Subscription) Close() error { s.once.Do(s.close); return nil }

// Subscribe captures current health and future changes without starting a goroutine.
func (c *Client) Subscribe() (*Subscription, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.subscribers) >= MaxSubscriptions {
		return nil, ErrUnavailable
	}
	channel := make(chan Change, 1)
	c.subscribers[channel] = true
	channel <- Change{Revision: c.state.Load().revision, Err: c.status.LastError}
	return &Subscription{Changes: channel, close: func() { c.mu.Lock(); defer c.mu.Unlock(); delete(c.subscribers, channel); close(channel) }}, nil
}

// Track submits native custom events only when the application selected an event sink.
func (c *Client) Track(ctx context.Context, name string, target Target, event Event) error {
	return submitEvent(ctx, name, target, nil, event, c.tracker)
}

// SupportsTracking reports whether the native engine has an explicit analytics destination.
func (c *Client) SupportsTracking() bool { return trackingEnabled(c.tracker) }

// changedKeys compares canonical flag definitions without exposing their values.
func changedKeys(before, after *compiled) []string {
	keys := map[string]bool{}
	for key, flag := range after.document.Flags {
		if before == nil {
			keys[key] = true
			continue
		}
		previous, present := before.document.Flags[key]
		old, _ := json.Marshal(encodedFlag(previous))
		next, _ := json.Marshal(encodedFlag(flag))
		if !present || !bytes.Equal(old, next) {
			keys[key] = true
		}
	}
	if before != nil {
		for key := range before.document.Flags {
			if _, present := after.document.Flags[key]; !present {
				keys[key] = true
			}
		}
	}
	result := make([]string, 0, len(keys))
	for key := range keys {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

// publishChange keeps subscribers nonblocking and notification errors separate from source health.
func (c *Client) publishChange(ctx context.Context, change Change) {
	c.mu.Lock()
	for channel := range c.subscribers {
		owned := change
		owned.Keys = append([]string(nil), change.Keys...)
		select {
		case channel <- owned:
		default:
			select {
			case <-channel:
			default:
			}
			owned.Keys = nil
			channel <- owned
		}
	}
	c.mu.Unlock()
	if c.notifier != nil {
		owned := change
		owned.Keys = append([]string(nil), change.Keys...)
		err := c.notifier.Notify(ctx, owned)
		if err != nil {
			err = safeError(err)
		}
		c.mu.Lock()
		c.status.LastNotifyError = err
		c.mu.Unlock()
	}
}
