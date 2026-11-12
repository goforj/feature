//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/goforj/feature"
	"github.com/goforj/feature/driver/launchdarklyfeature"
	"github.com/goforj/feature/driver/openfeaturefeature"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/open-feature/go-sdk-contrib/providers/launchdarkly/pkg"
)

// relayAdapter constructs each adapter over a real caller-owned SDK connected exclusively to the Relay.
func relayAdapter(t *testing.T, service *launchDarklyFixture, name string) (*feature.Features, *ldclient.LDClient) {
	t.Helper()
	config := service.sdkConfig()
	config.Events = ldcomponents.SendEvents().FlushInterval(time.Hour)
	sdk, err := ldclient.MakeCustomClient(evaluationKey, config, 2*time.Second)
	if sdk != nil {
		t.Cleanup(func() {
			if err := sdk.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	if err != nil {
		t.Fatal(err)
	}
	if name == "direct" {
		driver := launchdarklyfeature.New(sdk)
		t.Cleanup(func() {
			if err := driver.Close(); err != nil {
				t.Error(err)
			}
		})
		return feature.New(driver), sdk
	}
	driver, err := openfeaturefeature.Open(context.Background(), launchdarkly.NewProvider(sdk))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := driver.Close(); err != nil {
			t.Error(err)
		}
	})
	return feature.New(driver), sdk
}

// TestLaunchDarklyRelayContexts checks actual wire privacy and native identity semantics through both adapters.
func TestLaunchDarklyRelayContexts(t *testing.T) {
	service := launchDarklyRelay(t)
	for _, name := range []string{"direct", "openfeature"} {
		t.Run(name, func(t *testing.T) {
			prefix := namespace()
			service.publish(
				relayFlag(prefix+"multi", false).AddContextTarget("organization", 1, "tenant:acme").Build(),
				relayFlag(prefix+"typed", false).AddRule(ldbuilders.NewRuleBuilder().ID("paid-seats").Clauses(
					ldbuilders.Clause("paid", ldmodel.OperatorIn, ldvalue.Bool(true)),
					ldbuilders.Clause("seats", ldmodel.OperatorGreaterThanOrEqual, ldvalue.Int(10)),
				).Variation(1)).Build(),
				relayFlag(prefix+"private", true).TrackEvents(true).Build(),
				relayFlag(prefix+"off", false).On(false).OffVariation(1).Build(),
			)
			flags, sdk := relayAdapter(t, service, name)
			ctx := context.Background()
			eventually(t, func() bool { return flags.For(feature.Target{Key: "probe"}).Enabled(ctx, prefix+"private") })
			t.Run("multi-identity", func(t *testing.T) {
				target := feature.Target{Kind: "multi", Attributes: map[string]any{
					"user": map[string]any{"key": "user:7"}, "organization": map[string]any{"key": "tenant:acme"},
				}}
				if d := flags.For(target).Evaluate(ctx, prefix+"multi", false); !d.Value || d.Err != nil {
					t.Fatal(d)
				}
				target.Attributes["organization"] = map[string]any{"key": "tenant:other"}
				if d := flags.For(target).Evaluate(ctx, prefix+"multi", true); d.Value || d.Err != nil {
					t.Fatal("organization key was not authoritative", d)
				}
			})
			t.Run("typed-targeting", func(t *testing.T) {
				for _, seats := range []int{9, 12} {
					target := feature.Target{Key: "tenant:acme", Attributes: map[string]any{"paid": true, "seats": seats}}
					if d := flags.For(target).Evaluate(ctx, prefix+"typed", false); d.Value != (seats >= 10) || d.Err != nil {
						t.Fatal(d)
					}
				}
			})
			t.Run("off-variation", func(t *testing.T) {
				d := flags.For(feature.Target{Key: "tenant:acme"}).Evaluate(ctx, prefix+"off", false)
				if !d.Value || d.Err != nil || d.Reason != feature.ReasonDisabled {
					t.Fatal(d)
				}
			})
			t.Run("privacy", func(t *testing.T) {
				privateKey, publicKey := prefix+"private-user", prefix+"public-user"
				secret := privateKey + "@example.invalid"
				service.mu.Lock()
				service.privateLeaks[secret] = false
				service.mu.Unlock()
				private := feature.Target{Key: privateKey, Attributes: map[string]any{"email": secret, "plan": "pro", "privateAttributes": []string{"email"}}}
				public := feature.Target{Key: publicKey, Attributes: map[string]any{"email": "public@example.invalid", "plan": "pro"}}
				if !flags.For(private).Enabled(ctx, prefix+"private") || !flags.For(public).Enabled(ctx, prefix+"private") {
					t.Fatal("privacy metadata changed evaluation")
				}
				if !sdk.FlushAndWait(2 * time.Second) {
					t.Fatal("SDK analytics flush timed out")
				}
				eventually(t, func() bool {
					service.mu.Lock()
					defer service.mu.Unlock()
					return service.contexts[privateKey] != nil && service.contexts[publicKey] != nil
				})
				service.mu.Lock()
				privateJSON, publicJSON, leaked := service.contexts[privateKey], service.contexts[publicKey], service.privateLeaks[secret]
				service.mu.Unlock()
				var privateContext struct {
					Email *string
					Plan  string
					Meta  struct {
						Redacted []string `json:"redactedAttributes"`
					} `json:"_meta"`
				}
				var publicContext struct{ Email, Plan string }
				if err := json.Unmarshal(privateJSON, &privateContext); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(publicJSON, &publicContext); err != nil {
					t.Fatal(err)
				}
				if leaked || privateContext.Email != nil || privateContext.Plan != "pro" {
					t.Fatal("private attribute reached the collector or public metadata was lost")
				}
				if publicContext.Email != "public@example.invalid" || publicContext.Plan != "pro" {
					t.Fatal("collector did not receive the unredacted control")
				}
				if len(privateContext.Meta.Redacted) != 1 || privateContext.Meta.Redacted[0] != "email" {
					t.Fatal("native redaction metadata was lost", privateContext.Meta.Redacted)
				}
			})
		})
	}
}
