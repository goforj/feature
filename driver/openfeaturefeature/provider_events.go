package openfeaturefeature

import (
	"context"

	"github.com/goforj/feature"
	"github.com/open-feature/go-sdk/openfeature"
)

// changeSource exposes bounded native subscriptions without requiring a concrete engine.
type changeSource interface {
	Subscribe() (*feature.Subscription, error)
}

// EventChannel lets the SDK own its standard provider event dispatch.
func (p *Provider) EventChannel() <-chan openfeature.Event { return p.events }

// startEvents attaches one listener without starting another source refresh loop.
func (p *Provider) startEvents(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed.Load() {
		return feature.ErrClosed
	}
	if p.started {
		return nil
	}
	source, ok := p.evaluator.(changeSource)
	if !ok {
		p.started = true
		return nil
	}
	subscription, err := source.Subscribe()
	if err != nil {
		return feature.ErrUnavailable
	}
	p.subscription = subscription
	p.stop = make(chan struct{})
	p.done = make(chan struct{})
	p.started = true
	go p.forwardChanges(subscription)
	return nil
}

// forwardChanges preserves health transitions while native listeners coalesce slow consumers.
func (p *Provider) forwardChanges(subscription *feature.Subscription) {
	defer close(p.done)
	initial, stale := true, false
	for {
		select {
		case <-p.stop:
			return
		case change, open := <-subscription.Changes:
			if !open {
				return
			}
			if initial && change.Err == nil && change.PreviousRevision == "" {
				initial = false
				continue
			}
			initial = false
			if change.Err != nil {
				stale = true
				if !p.emitChange(openfeature.ProviderStale, change) {
					return
				}
				continue
			}
			if stale {
				stale = false
				if !p.emitChange(openfeature.ProviderReady, change) {
					return
				}
			}
			if !p.emitChange(openfeature.ProviderConfigChange, change) {
				return
			}
		}
	}
}

// emitChange bounds queued SDK events and makes blocked dispatch interruptible at shutdown.
func (p *Provider) emitChange(kind openfeature.EventType, change feature.Change) bool {
	event := openfeature.Event{ProviderName: p.Metadata().Name, EventType: kind, ProviderEventDetails: openfeature.ProviderEventDetails{FlagChanges: append([]string(nil), change.Keys...), EventMetadata: map[string]any{"revision": change.Revision}}}
	if change.Err != nil {
		event.Message = "feature: source refresh failed"
		event.ErrorCode = openfeature.GeneralCode
	}
	select {
	case <-p.stop:
		return false
	case p.events <- event:
		return true
	}
}

// stopEvents waits for owned forwarding while leaving the source engine and SDK resources borrowed.
func (p *Provider) stopEvents() {
	p.once.Do(func() {
		p.closed.Store(true)
		p.mu.Lock()
		stop, done, subscription := p.stop, p.done, p.subscription
		p.mu.Unlock()
		if stop != nil {
			close(stop)
		}
		if subscription != nil {
			_ = subscription.Close()
		}
		if done != nil {
			<-done
		}
	})
}

var _ openfeature.EventHandler = (*Provider)(nil)
var _ openfeature.Tracker = (*Provider)(nil)
