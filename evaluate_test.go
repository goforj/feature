package feature

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// TestEvaluation covers defaults, all conditions, ordering, and explicit fallback reasons.
func TestEvaluation(t *testing.T) {
	ctx := context.Background()
	doc := sampleDocument()
	doc.Flags["enabled"] = Flag{Default: true}
	doc.Flags["disabled"] = Flag{Default: true, Disabled: true}
	doc.Flags["ordered"] = Flag{Rules: []Rule{{Subjects: []string{"staff"}, Value: false}, {Attributes: map[string]string{"plan": "pro"}, Value: true}}}
	doc.Flags["attrs"] = Flag{Rules: []Rule{{Attributes: map[string]string{"plan": ""}, Value: true}}}
	state, err := compile(doc)
	if err != nil {
		t.Fatal(err)
	}
	v := &View{state: state}
	cases := []struct {
		name, key      string
		subject        Subject
		fallback, want bool
		reason         Reason
		err            error
	}{
		{"default true", "enabled", Subject{}, false, true, ReasonDefault, nil},
		{"default false", "checkout-v2", Subject{}, true, false, ReasonDefault, nil},
		{"disabled", "disabled", Subject{}, true, false, ReasonDisabled, nil},
		{"first subject", "checkout-v2", Subject{Key: "tenant:internal"}, false, true, ReasonRule, nil},
		{"second subject", "checkout-v2", Subject{Key: "tenant:staff"}, false, true, ReasonRule, nil},
		{"missing flag", "missing", Subject{}, true, true, ReasonMissing, ErrNotFound},
		{"invalid key", "bad key", Subject{}, true, true, ReasonError, ErrInvalidKey},
		{"invalid subject", "enabled", Subject{Key: strings.Repeat("x", MaxStringBytes+1)}, false, false, ReasonError, ErrInvalidSubject},
		{"invalid attributes", "enabled", Subject{Attributes: map[string]string{"bad key": ""}}, false, false, ReasonError, ErrInvalidSubject},
		{"ordered false", "ordered", Subject{Key: "staff", Attributes: map[string]string{"plan": "pro"}}, true, false, ReasonRule, nil},
		{"attribute only", "ordered", Subject{Attributes: map[string]string{"plan": "pro"}}, false, true, ReasonRule, nil},
		{"missing attribute", "attrs", Subject{}, true, false, ReasonDefault, nil},
		{"empty attribute", "attrs", Subject{Attributes: map[string]string{"plan": ""}}, false, true, ReasonRule, nil},
		{"different attribute", "ordered", Subject{Attributes: map[string]string{"plan": "PRO"}}, true, false, ReasonDefault, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := v.Evaluate(ctx, tc.key, tc.fallback, tc.subject)
			if d.Value != tc.want || d.Reason != tc.reason || d.Err != tc.err || d.Revision != state.revision {
				t.Fatalf("%+v", d)
			}
		})
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	d := v.Evaluate(canceled, "enabled", false, Subject{})
	if d.Err != context.Canceled || d.Value {
		t.Fatal(d)
	}
}

// TestRolloutGoldenVectors freezes the wire-independent v1 assignment contract.
func TestRolloutGoldenVectors(t *testing.T) {
	cases := []struct {
		key, salt, subject string
		want               uint64
	}{
		{"checkout-v2", "checkout-2026", "tenant:acme", 6886},
		{"checkout-v2", "checkout-2026", "tenant:internal", 4257},
		{"a", "bc", "d", 2887},
		{"ab", "c", "d", 6212},
		{"checkout-v2", "new-salt", "tenant:acme", 822},
		{"x", "salt", "tenant:東京", 4310},
	}
	for _, tc := range cases {
		if got := bucket(tc.key, tc.salt, tc.subject); got != tc.want {
			t.Fatalf("bucket=%d want %d", got, tc.want)
		}
	}
}

// TestRolloutMembership verifies boundaries, monotonic inclusion, and stable repeated reads.
func TestRolloutMembership(t *testing.T) {
	for i := 0; i < 1000; i++ {
		subject := Subject{Key: fmt.Sprintf("tenant:%d", i)}
		r := Rule{Rollout: &Rollout{Salt: "stable", BasisPoints: 0}}
		if matches(r, "checkout", subject) {
			t.Fatal("zero rollout matched")
		}
		r.Rollout.BasisPoints = 10000
		if !matches(r, "checkout", subject) || matches(r, "checkout", Subject{}) {
			t.Fatal("identity boundary failed")
		}
		r.Rollout.BasisPoints = 1000
		first := matches(r, "checkout", subject)
		if matches(r, "checkout", subject) != first {
			t.Fatal("unstable decision")
		}
		r.Rollout.BasisPoints = 5000
		if first && !matches(r, "checkout", subject) {
			t.Fatal("membership shrank")
		}
	}
}
