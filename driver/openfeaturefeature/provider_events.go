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
		return p.readinessError()
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
	return p.readinessError()
}

// readinessError prevents SDK initialization from declaring an unhealthy native source ready.
func (p *Provider) readinessError() error {
	if source, ok := p.evaluator.(interface{ Status() feature.Status }); ok && source.Status().LastError != nil {
		p.initFailed.Store(true)
		return feature.ErrUnavailable
	}
	return nil
}

// forwardChanges preserves health transitions while native listeners coalesce slow consumers.
func (p *Provider) forwardChanges(subscription *feature.Subscription) {
	defer close(p.done)
	initial, stale := true, false
	revision := ""
	for {
		select {
		case <-p.stop:
			return
		case change, open := <-subscription.Changes:
			if !open {
				return
			}
			if initial && change.Err == nil && change.PreviousRevision == "" && !p.initFailed.Load() {
				initial = false
				revision = change.Revision
				continue
			}
			initial = false
			if change.Err != nil {
				if change.Revision != revision {
					if !p.emitChange(openfeature.ProviderConfigChange, change) {
						return
					}
					revision = change.Revision
				}
				stale = true
				if !p.emitChange(openfeature.ProviderStale, change) {
					return
				}
				continue
			}
			initFailed := p.initFailed.Swap(false)
			if stale || initFailed {
				stale = false
				if !p.emitChange(openfeature.ProviderReady, change) {
					return
				}
			}
			if !p.emitChange(openfeature.ProviderConfigChange, change) {
				return
			}
			revision = change.Revision
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
