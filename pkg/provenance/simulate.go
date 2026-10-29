package provenance

import (
	"context"
	"slices"
	"time"
)

// Simulation replays recorded events against a policy: "run this policy
// against the last 30 days". It uses each event's recorded action, the labels
// found in its content, its lineage and the session labels at the time, so a
// policy is tested on what really happened before it is enforced.

const (
	simulationBatch   = 500
	simulationSamples = 50
	simulationMax     = 200000
)

// SimulationResult is what a policy would have done.
type SimulationResult struct {
	Policy   Policy    `json:"policy"`
	Since    time.Time `json:"since"`
	Until    time.Time `json:"until"`
	Examined int       `json:"examined"`
	Matched  int       `json:"matched"`
	// ByEffect counts what the matches would have done, which for a judge
	// effect is "judge" since the Supervisor is not consulted in simulation.
	ByEffect map[Effect]int `json:"byEffect"`
	// Changed counts matches where the recorded decision differed: the policy
	// would have blocked something that was allowed, or the reverse.
	Changed int `json:"changed"`
	// Sessions is how many distinct sessions the matches span.
	Sessions int `json:"sessions"`
	// Users is how many distinct users.
	Users int `json:"users"`
	// Samples are up to 50 matched events, most recent first.
	Samples []Event `json:"samples"`
	// Errors counts events whose condition failed to evaluate.
	Errors int `json:"errors"`
	// FalsePositiveHint is the fraction of matches on sessions that never
	// tripped any enforced policy, as a rough proxy for noise.
	FalsePositiveHint float64 `json:"falsePositiveHint"`
}

// Simulate replays events from since against a policy.
func (e *Engine) Simulate(ctx context.Context, p Policy, since time.Time, surfaces []Surface) (*SimulationResult, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	settings := e.Settings(ctx)
	now := e.now()
	res := &SimulationResult{Policy: p, Since: since, Until: now, ByEffect: map[Effect]int{}}
	sessions := map[string]bool{}
	users := map[string]bool{}
	quiet := map[string]bool{}
	matchedOnQuiet := 0

	sessionCache := map[string]*Session{}
	offset := 0
	for res.Examined < simulationMax {
		events, _, err := e.store.ListEvents(ctx, EventQuery{Start: since, Limit: simulationBatch, Offset: offset, Decided: new(true)})
		if err != nil {
			return nil, err
		}
		if len(events) == 0 {
			break
		}
		offset += len(events)
		for _, ev := range events {
			if ev.Action == nil || ev.Decision == nil {
				continue
			}
			if len(surfaces) > 0 && !containsSurface(surfaces, ev.Surface) {
				continue
			}
			if !p.AppliesTo(ev.Surface) {
				continue
			}
			res.Examined++
			s, ok := sessionCache[ev.SessionID]
			if !ok {
				s, _ = e.store.GetSession(ctx, ev.SessionID)
				if s == nil {
					s = &Session{ID: ev.SessionID, UserID: ev.UserID}
				}
				sessionCache[ev.SessionID] = s
			}
			in := ReplayInput(ev, s, settings)
			ok, err := EvalPolicy(&p, in)
			if err != nil {
				res.Errors++
				continue
			}
			if !ok {
				continue
			}
			res.Matched++
			res.ByEffect[p.Effect]++
			sessions[ev.SessionID] = true
			if ev.UserID != "" {
				users[ev.UserID] = true
			}
			wouldBlock := p.Effect.Blocks() || p.Effect == EffectJudge
			if wouldBlock == ev.Decision.Allow {
				res.Changed++
			}
			if _, seen := quiet[ev.SessionID]; !seen {
				quiet[ev.SessionID] = s.State.Denied == 0
			}
			if quiet[ev.SessionID] {
				matchedOnQuiet++
			}
			if len(res.Samples) < simulationSamples {
				sample := ev
				sample.Action = contentFree(ev.Action)
				res.Samples = append(res.Samples, sample)
			}
		}
	}
	res.Sessions = len(sessions)
	res.Users = len(users)
	if res.Matched > 0 {
		res.FalsePositiveHint = float64(matchedOnQuiet) / float64(res.Matched)
	}
	return res, nil
}

// ReplayInput rebuilds the policy input for a recorded event. The session's
// labels and risk come from the decision recorded at the time, so the replay
// sees the session as it was, not as it is now.
func ReplayInput(ev Event, s *Session, settings Settings) PolicyInput {
	at := Session{ID: s.ID, UserID: s.UserID, Email: s.Email, Agent: s.Agent, Actor: s.Actor, StartedAt: s.StartedAt}
	at.State.Labels = ev.Decision.Labels
	at.State.Risk = ev.Decision.Risk
	at.State.Killed = ev.Decision.Killed
	at.State.Warrant = s.State.Warrant
	at.State.Surfaces = s.State.Surfaces
	subject := Subject{UserID: ev.UserID, Email: s.Email, Actor: ev.Actor}
	action := *ev.Action
	return PolicyInput{
		Subject: subject, Action: action, Session: &at, Settings: settings, Now: ev.At,
		ActionLabels: ev.Labels, Lineage: ev.Lineage, Honeytoken: ev.Decision.Honeytoken,
	}
}

func contentFree(a *Action) *Action {
	if a == nil {
		return nil
	}
	c := *a
	c.Text, c.Arguments = "", nil
	return &c
}

func containsSurface(list []Surface, s Surface) bool {
	return slices.Contains(list, s)
}
