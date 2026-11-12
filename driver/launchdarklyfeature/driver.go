// Package launchdarklyfeature adapts the official LaunchDarkly server SDK.
// New borrows an initialized SDK client. Open owns a client and its update/event
// lifecycle. Both preserve native SDK evaluation and configured off-variations.
package launchdarklyfeature

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/goforj/feature"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7"
)

// ErrInvalidConfig identifies invalid startup policy without exposing SDK credentials.
var ErrInvalidConfig = errors.New("launchdarklyfeature: invalid configuration")

// Client is the consumer-owned portion of the native SDK used for boolean evaluation.
type Client interface {
	BoolVariationDetailCtx(context.Context, string, ldcontext.Context, bool) (bool, ldreason.EvaluationDetail, error)
}

// managedClient adds the lifecycle owned exclusively by Open.
type managedClient interface {
	Client
	Initialized() bool
	Close() error
}

// Config supplies explicit SDK credentials, native options, and bounded initialization.
// SDKConfig retains native transport, privacy, event, and data-source configuration.
// InitTimeout defaults to five seconds; the caller's deadline can shorten it.
// Synchronous SDK component factories must enforce their own I/O deadlines.
type Config struct {
	SDKKey      string
	SDKConfig   ldclient.Config
	InitTimeout time.Duration
}

// Driver borrows or owns an SDK client and supports concurrent evaluation and shutdown.
type Driver struct {
	client   Client
	close    func() error
	closed   atomic.Bool
	once     sync.Once
	closeErr error
}

// New borrows an initialized native client without changing its configuration or lifecycle.
func New(client Client) *Driver { return &Driver{client: client} }

// Open starts the real SDK and waits for initialization without global registration.
// Cancellation closes a partially initialized client; successful drivers own shutdown.
func Open(ctx context.Context, config Config) (*Driver, error) {
	return open(ctx, config, func(key string, config ldclient.Config) (managedClient, error) {
		return ldclient.MakeCustomClient(key, config, 0)
	})
}

// open makes initialization and partial-start cleanup independently testable.
func open(ctx context.Context, config Config, create func(string, ldclient.Config) (managedClient, error)) (*Driver, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.SDKKey) == "" || config.InitTimeout < 0 {
		return nil, ErrInvalidConfig
	}
	timeout := config.InitTimeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	client, err := create(config.SDKKey, config.SDKConfig)
	if err != nil {
		if client != nil {
			_ = client.Close()
		}
		if err := waitCtx.Err(); err != nil {
			return nil, err
		}
		return nil, feature.ErrUnavailable
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for !client.Initialized() {
		select {
		case <-waitCtx.Done():
			_ = client.Close()
			return nil, waitCtx.Err()
		case <-ticker.C:
		}
	}
	if err := waitCtx.Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return &Driver{client: client, close: client.Close}, nil
}

// Close disables this adapter once and closes only SDK resources created by Open.
// The native SDK controls its flush and close timeout policy through SDKConfig.
// Concurrent callers wait for the same cleanup and receive the same safe result.
func (d *Driver) Close() error {
	d.once.Do(func() {
		d.closed.Store(true)
		if d.close != nil {
			if err := d.close(); err != nil {
				d.closeErr = feature.ErrUnavailable
			}
		}
	})
	return d.closeErr
}

// Resolve preserves native values, variation indexes, and safe error classifications.
func (d *Driver) Resolve(ctx context.Context, key string, fallback bool, target feature.Target) feature.Decision {
	result := feature.Decision{Value: fallback, Reason: feature.ReasonError}
	if err := ctx.Err(); err != nil {
		result.Err = err
		return result
	}
	if d.closed.Load() {
		result.Err = feature.ErrClosed
		return result
	}
	owned, err := target.Clone()
	if err != nil {
		result.Err = err
		return result
	}
	native, err := toContext(owned)
	if err != nil {
		result.Err = err
		return result
	}
	value, detail, err := d.client.BoolVariationDetailCtx(ctx, key, native, fallback)
	if err != nil || detail.Reason.GetKind() == ldreason.EvalReasonError {
		result.Err = resolutionError(detail.Reason.GetErrorKind())
		if result.Err == feature.ErrNotFound {
			result.Reason = feature.ReasonMissing
		}
		if ctx.Err() != nil {
			result.Err, result.Reason = ctx.Err(), feature.ReasonError
		}
		return result
	}
	result.Value = value
	switch detail.Reason.GetKind() {
	case ldreason.EvalReasonOff:
		result.Reason = feature.ReasonDisabled
	case ldreason.EvalReasonTargetMatch, ldreason.EvalReasonRuleMatch:
		result.Reason = feature.ReasonRule
	case ldreason.EvalReasonFallthrough:
		result.Reason = feature.ReasonDefault
	default:
		result.Reason = feature.ReasonUnknown
	}
	if detail.VariationIndex.IsDefined() {
		result.Variant = detail.VariationIndex.String()
	}
	return result
}

// resolutionError keeps SDK error prose and flag data out of returned errors.
func resolutionError(kind ldreason.EvalErrorKind) error {
	switch kind {
	case ldreason.EvalErrorFlagNotFound:
		return feature.ErrNotFound
	case ldreason.EvalErrorWrongType:
		return feature.ErrTypeMismatch
	case ldreason.EvalErrorUserNotSpecified:
		return feature.ErrInvalidSubject
	default:
		return feature.ErrUnavailable
	}
}

// toContext supports explicit single identities and LaunchDarkly's structured multi convention.
func toContext(target feature.Target) (ldcontext.Context, error) {
	if target.Kind != "multi" {
		return singleContext(target.Key, target.Kind, target.Attributes)
	}
	builder := ldcontext.NewMultiBuilder()
	for kind, value := range target.Attributes {
		attributes, ok := value.(map[string]any)
		if !ok {
			return ldcontext.Context{}, feature.ErrInvalidSubject
		}
		key, ok := attributes["key"].(string)
		if !ok {
			return ldcontext.Context{}, feature.ErrInvalidSubject
		}
		identity, err := singleContext(key, kind, attributes)
		if err != nil {
			return ldcontext.Context{}, err
		}
		builder.Add(identity)
	}
	native := builder.Build()
	if native.Err() != nil {
		return ldcontext.Context{}, feature.ErrInvalidSubject
	}
	return native, nil
}

// singleContext preserves native identity kinds, typed attributes, and explicit privacy metadata.
func singleContext(key, kind string, attributes map[string]any) (ldcontext.Context, error) {
	if kind == "" {
		kind = "user"
	}
	builder := ldcontext.NewBuilder(key).Kind(ldcontext.Kind(kind))
	for name, value := range attributes {
		switch name {
		case "key", "kind", "targetingKey":
			continue
		case "anonymous":
			anonymous, ok := value.(bool)
			if !ok {
				return ldcontext.Context{}, feature.ErrInvalidSubject
			}
			builder.Anonymous(anonymous)
		case "privateAttributes":
			private, ok := value.([]string)
			if !ok {
				return ldcontext.Context{}, feature.ErrInvalidSubject
			}
			builder.Private(private...)
		default:
			builder.SetValue(name, ldvalue.CopyArbitraryValue(value))
		}
	}
	native := builder.Build()
	if native.Err() != nil {
		return ldcontext.Context{}, feature.ErrInvalidSubject
	}
	return native, nil
}
