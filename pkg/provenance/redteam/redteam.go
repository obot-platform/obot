// Package redteam is Obot's continuous assurance harness. It drives the
// provenance engine (not the live surfaces) through the ten technical attack
// scenarios from the market brief and scores each on four axes: Observed (the
// engine recorded the action), Detected (a policy matched, shadow or enforced),
// Blocked (the decision was not allowed) and Remediated (an approval or kill
// path exists for it). The result is an honest scorecard: with the built-in
// policies in shadow mode most scenarios are Observed and Detected but not
// Blocked, and the Notes say so.
//
// The harness is safe to run against a live deployment. Every scenario uses a
// synthetic user and session key prefixed "redteam-", so it never joins a real
// session, and the one honeytoken it plants for the exfiltration scenario is
// removed afterward.
package redteam

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/obot-platform/obot/pkg/provenance"
)

const (
	// Prefix scopes every synthetic identifier the harness creates.
	Prefix = "redteam-"
	// honeytokenValue is the canary planted and then matched by the
	// exfiltration scenario. It is a syntactically valid but fake AWS key.
	honeytokenValue = "AKIAREDTEAMCANARY999"
)

// Options configures a run.
type Options struct {
	// UserID and Email identify the synthetic subject. Defaults are derived
	// from Prefix.
	UserID string
	Email  string
	// Agent is the synthetic agent name. Default "redteam-agent".
	Agent string
	// Only, when set, limits the run to the named scenarios.
	Only []string
	// KeepArtifacts leaves the planted honeytoken in place (for debugging).
	// By default it is removed after the run.
	KeepArtifacts bool
}

// ScenarioResult is the score for one scenario on the surface it exercises.
type ScenarioResult struct {
	Name       string             `json:"name"`
	Surface    provenance.Surface `json:"surface"`
	Observed   bool               `json:"observed"`
	Detected   bool               `json:"detected"`
	Blocked    bool               `json:"blocked"`
	Remediated bool               `json:"remediated"`
	Notes      string             `json:"notes"`
}

// Summary counts the scenarios that reached each axis.
type Summary struct {
	Scenarios  int `json:"scenarios"`
	Observed   int `json:"observed"`
	Detected   int `json:"detected"`
	Blocked    int `json:"blocked"`
	Remediated int `json:"remediated"`
}

// Scorecard is the result of a run.
type Scorecard struct {
	RunAt     time.Time        `json:"runAt"`
	Scenarios []ScenarioResult `json:"scenarios"`
	Summary   Summary          `json:"summary"`
}

// scenario is one attack the harness runs against the engine.
type scenario struct {
	name    string
	surface provenance.Surface
	run     func(ctx context.Context, h *harness) ScenarioResult
}

// harness holds the per-run state: the engine, the synthetic subject, and a
// unique session key per scenario so scenarios never pollute each other or
// real data.
type harness struct {
	engine  *provenance.Engine
	subject provenance.Subject
	agent   string
	runID   string
	seq     int
}

// Scenarios returns the names of every scenario, in run order.
func Scenarios() []string {
	names := make([]string, 0, len(allScenarios))
	for _, s := range allScenarios {
		names = append(names, s.name)
	}
	return names
}

// Run drives the engine through the scenarios and returns the scorecard.
func Run(ctx context.Context, engine *provenance.Engine, opts Options) Scorecard {
	h := newHarness(engine, opts)
	card := Scorecard{RunAt: h.now()}
	only := map[string]bool{}
	for _, n := range opts.Only {
		only[n] = true
	}
	for _, sc := range allScenarios {
		if len(only) > 0 && !only[sc.name] {
			continue
		}
		res := sc.run(ctx, h)
		res.Name = sc.name
		if res.Surface == "" {
			res.Surface = sc.surface
		}
		card.Scenarios = append(card.Scenarios, res)
	}
	card.Summary = summarize(card.Scenarios)
	return card
}

func summarize(results []ScenarioResult) Summary {
	s := Summary{Scenarios: len(results)}
	for _, r := range results {
		if r.Observed {
			s.Observed++
		}
		if r.Detected {
			s.Detected++
		}
		if r.Blocked {
			s.Blocked++
		}
		if r.Remediated {
			s.Remediated++
		}
	}
	return s
}

func newHarness(engine *provenance.Engine, opts Options) *harness {
	userID := firstNonEmpty(opts.UserID, Prefix+"user")
	agent := firstNonEmpty(opts.Agent, Prefix+"agent")
	return &harness{
		engine: engine,
		subject: provenance.Subject{
			UserID: userID,
			Email:  firstNonEmpty(opts.Email, Prefix+"user@redteam.invalid"),
			Groups: []string{"redteam"},
			Actor:  provenance.Actor{Agent: agent, DeviceID: Prefix + "device"},
		},
		agent: agent,
		runID: provenance.NewID("rt"),
	}
}

func (h *harness) now() time.Time { return time.Now().UTC() }

// key returns a fresh session key for a scenario, scoped to this run so no two
// scenarios stitch together and nothing joins a real session.
func (h *harness) key(scenario string) provenance.SessionKey {
	h.seq++
	return provenance.SessionKey{
		ClientSessionID: fmt.Sprintf("%s%s-%s-%d", Prefix, h.runID, scenario, h.seq),
		Agent:           h.agent,
	}
}

// ingest records content entering a scenario's session.
func (h *harness) ingest(ctx context.Context, key provenance.SessionKey, in provenance.IngestRequest) (*provenance.Event, error) {
	in.Subject = h.subject
	in.Session = key
	return h.engine.Ingest(ctx, in)
}

// decide decides an action in a scenario's session.
func (h *harness) decide(ctx context.Context, key provenance.SessionKey, a provenance.Action) (*provenance.Event, provenance.Decision, error) {
	return h.engine.Decide(ctx, provenance.DecideRequest{Subject: h.subject, Session: key, Action: a})
}

// score builds a result from a decision and whether the event was recorded.
// Detected is true when any policy matched (shadow or enforced); Blocked is
// true when the decision did not allow the action; Remediated is true when an
// approval was created or the action would be held or killed.
func score(surface provenance.Surface, observed bool, d provenance.Decision) ScenarioResult {
	detected := len(d.Matched) > 0 || d.Honeytoken || d.Killed
	blocked := !d.Allow
	remediated := d.ApprovalID != "" || d.Killed || wouldHold(d)
	return ScenarioResult{
		Surface: surface, Observed: observed, Detected: detected, Blocked: blocked, Remediated: remediated,
		Notes: notesFor(d),
	}
}

// wouldHold reports whether the strongest matched effect is one a person or the
// kill switch can resolve, even if it was only shadowed.
func wouldHold(d provenance.Decision) bool {
	for _, m := range d.Matched {
		switch m.Effect {
		case provenance.EffectRequireApproval, provenance.EffectDeny, provenance.EffectJudge:
			return true
		}
	}
	return d.Effect.Blocks()
}

func notesFor(d provenance.Decision) string {
	switch {
	case d.Honeytoken:
		return "honeytoken match: definitive exfiltration evidence, denied in enforce"
	case d.Killed:
		return "denied by the kill switch"
	case len(d.Matched) == 0:
		return "no policy matched this action"
	case d.Shadow && !d.Allow:
		return "detected and blocked by an enforced policy"
	case d.Shadow:
		return fmt.Sprintf("detected by %s in shadow mode: recorded as %q but allowed; enable enforce to block", matchedNames(d), d.Effect)
	case !d.Allow:
		return fmt.Sprintf("blocked by %s (%s)", d.PolicyName, d.Effect)
	default:
		return fmt.Sprintf("allowed; matched %s", matchedNames(d))
	}
}

func matchedNames(d provenance.Decision) string {
	var names []string
	for _, m := range d.Matched {
		name := m.PolicyName
		if m.Shadow {
			name += " (shadow)"
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return "no policy"
	}
	return strings.Join(names, ", ")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
