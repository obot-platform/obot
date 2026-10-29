package redteam

import (
	"context"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/provenance"
)

func newEngine(t *testing.T) *provenance.Engine {
	t.Helper()
	e := provenance.New(provenance.NewMemStore())
	if err := e.Seed(t.Context()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return e
}

func TestRunScorecardShape(t *testing.T) {
	ctx := context.Background()
	e := newEngine(t)
	card := Run(ctx, e, Options{})

	if len(card.Scenarios) != len(allScenarios) {
		t.Fatalf("expected %d scenarios, got %d", len(allScenarios), len(card.Scenarios))
	}
	if card.RunAt.IsZero() {
		t.Fatal("RunAt should be set")
	}
	if card.Summary.Scenarios != len(card.Scenarios) {
		t.Fatalf("summary scenario count: %d", card.Summary.Scenarios)
	}

	names := map[string]ScenarioResult{}
	for _, r := range card.Scenarios {
		if r.Name == "" {
			t.Fatal("every scenario result needs a name")
		}
		if r.Surface == "" {
			t.Fatalf("scenario %q has no surface", r.Name)
		}
		if r.Notes == "" {
			t.Fatalf("scenario %q has no notes", r.Name)
		}
		names[r.Name] = r
	}

	// The exfiltration honeytoken policy ships enforced, so that scenario is
	// blocked out of the box.
	exfil, ok := names["outbound-exfiltration"]
	if !ok {
		t.Fatal("missing outbound-exfiltration scenario")
	}
	if !exfil.Observed || !exfil.Detected || !exfil.Blocked {
		t.Fatalf("exfiltration should be observed, detected and blocked out of the box: %+v", exfil)
	}

	// With the rest of the built-ins in shadow mode, the summary must have
	// more detections than blocks: honest reporting of shadow mode.
	if card.Summary.Detected <= card.Summary.Blocked {
		t.Fatalf("in shadow mode detections (%d) should exceed blocks (%d)", card.Summary.Detected, card.Summary.Blocked)
	}
	if card.Summary.Observed < card.Summary.Detected {
		t.Fatalf("observed (%d) should be at least detected (%d)", card.Summary.Observed, card.Summary.Detected)
	}

	// The honeytoken must be cleaned up after the run.
	tokens, err := e.Store().ListHoneytokens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tok := range tokens {
		if tok.Name == Prefix+"canary" {
			t.Fatalf("the planted honeytoken was not removed: %+v", tok)
		}
	}
}

func TestEnforceRaisesBlockedCount(t *testing.T) {
	ctx := context.Background()

	shadow := Run(ctx, newEngine(t), Options{})

	// A fresh engine with every built-in flipped to enforce.
	enforced := newEngine(t)
	policies, err := enforced.Policies(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i := range policies {
		p := policies[i]
		p.Mode = provenance.ModeEnforce
		if err := enforced.Store().PutPolicy(ctx, &p); err != nil {
			t.Fatal(err)
		}
	}
	enforced.Invalidate()

	enforcedCard := Run(ctx, enforced, Options{})

	if enforcedCard.Summary.Blocked <= shadow.Summary.Blocked {
		t.Fatalf("enforcing built-ins should raise the blocked count: shadow %d, enforce %d",
			shadow.Summary.Blocked, enforcedCard.Summary.Blocked)
	}
	// Enforcing should not reduce what is detected.
	if enforcedCard.Summary.Detected < shadow.Summary.Detected {
		t.Fatalf("enforcing should not lower detections: shadow %d, enforce %d",
			shadow.Summary.Detected, enforcedCard.Summary.Detected)
	}
}

func TestOnlyFilter(t *testing.T) {
	ctx := context.Background()
	card := Run(ctx, newEngine(t), Options{Only: []string{"model-request", "outbound-exfiltration"}})
	if len(card.Scenarios) != 2 {
		t.Fatalf("expected 2 scenarios, got %d", len(card.Scenarios))
	}
}

func TestScenariosAreNamed(t *testing.T) {
	got := Scenarios()
	if len(got) != 10 {
		t.Fatalf("the brief lists 10 scenarios, got %d", len(got))
	}
	seen := map[string]bool{}
	for _, n := range got {
		if seen[n] {
			t.Fatalf("duplicate scenario name %q", n)
		}
		seen[n] = true
	}
}

func TestSyntheticKeysAreScoped(t *testing.T) {
	h := newHarness(newEngine(t), Options{})
	a := h.key("one")
	b := h.key("one")
	if a.ClientSessionID == b.ClientSessionID {
		t.Fatal("each scenario call should get a distinct session key")
	}
	for _, k := range []provenance.SessionKey{a, b} {
		if len(k.ClientSessionID) < len(Prefix) || k.ClientSessionID[:len(Prefix)] != Prefix {
			t.Fatalf("synthetic key must carry the redteam prefix: %q", k.ClientSessionID)
		}
	}
}

func TestRunRespectsDeadlineContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	card := Run(ctx, newEngine(t), Options{Only: []string{"model-request"}})
	if len(card.Scenarios) != 1 {
		t.Fatalf("expected 1 scenario, got %d", len(card.Scenarios))
	}
}
