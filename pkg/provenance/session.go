package provenance

import (
	"crypto/rand"
	"encoding/hex"
	"slices"
	"strings"
	"time"
)

const (
	// SessionIdleTimeout is how long a session may be quiet before activity under the
	// same fallback key starts a new one.
	SessionIdleTimeout = 30 * time.Minute
	// maxSessionFingerprints bounds the fingerprints a session remembers. The
	// oldest are dropped first; the session's labels (its taint) are never
	// dropped, so dropping fingerprints only loses precision, never safety.
	maxSessionFingerprints = 20000
	// maxSessionLabelOrigins bounds the per-label origin lists kept for
	// explanations.
	maxSessionLabelOrigins = 8
)

// Subject is who an action is performed for: the Obot user and the actor
// (agent, device, environment, key) acting on their behalf.
type Subject struct {
	UserID string   `json:"userID,omitempty"`
	Email  string   `json:"email,omitempty"`
	Groups []string `json:"groups,omitempty"`
	Actor  Actor    `json:"actor"`
}

// SessionKey is what a surface knows about which session an event belongs
// to. The engine resolves it to a session ID, creating a session when none
// matches. Fields are tried in order: an explicit ID, then the agent's own
// session identifier, then a fallback on the actor with an idle timeout.
type SessionKey struct {
	// Explicit is a session ID the surface already knows: one the engine issued
	// and the client propagated (X-Obot-Session-ID), or one the engine returned
	// from an earlier call.
	Explicit string `json:"explicit,omitempty"`
	// ClientSessionID is the agent's own session identifier: Claude Code's
	// session_id, an Mcp-Session-Id, a Claude Environment session, a Copilot
	// Studio conversation ID. Different agents use different ID spaces, so it
	// is scoped by Agent when set.
	ClientSessionID string `json:"clientSessionID,omitempty"`
	// Agent scopes ClientSessionID and the fallback key.
	Agent string `json:"agent,omitempty"`
	// UserID, DeviceID, APIKeyID and EnvironmentID build the fallback key when
	// no client session ID is known.
	UserID        string `json:"userID,omitempty"`
	DeviceID      string `json:"deviceID,omitempty"`
	APIKeyID      string `json:"apiKeyID,omitempty"`
	EnvironmentID string `json:"environmentID,omitempty"`
}

// LabelOrigin records where a session label came from: the event that
// introduced it and a short description, for explanations.
type LabelOrigin struct {
	EventID string    `json:"eventID"`
	Summary string    `json:"summary,omitempty"`
	At      time.Time `json:"at"`
}

// SessionState is everything the engine needs to decide an action in a
// session. It is the precomputed hot-path state: a decision never queries the
// event graph.
type SessionState struct {
	// Labels is the session's taint: every label that has entered its context.
	Labels Labels `json:"labels,omitempty"`
	// Origins explains each label with the events that introduced it.
	Origins map[Label][]LabelOrigin `json:"origins,omitempty"`
	// Fingerprints are the sensitive spans remembered for lineage matching.
	Fingerprints []Fingerprint `json:"fingerprints,omitempty"`
	// Risk is 0 to 100.
	Risk int `json:"risk"`
	// Counters.
	Events      int `json:"events"`
	ToolCalls   int `json:"toolCalls"`
	Denied      int `json:"denied"`
	Shadowed    int `json:"shadowed"`
	Approvals   int `json:"approvals"`
	ExternalOut int `json:"externalOut"`
	// Surfaces that have reported into the session, for coverage.
	Surfaces []Surface `json:"surfaces,omitempty"`
	// Warrant is the session's intent-bound authority, when one was issued.
	Warrant *Warrant `json:"warrant,omitempty"`
	// Killed reports that the session was killed.
	Killed bool `json:"killed,omitempty"`
	// Quarantined reports that untrusted content is being routed through the
	// quarantined model.
	Quarantined bool `json:"quarantined,omitempty"`
	// Downshifted reports that credentials have been downshifted to read-only.
	Downshifted bool `json:"downshifted,omitempty"`
	// LastSummary is the most recent event summary, for lists.
	LastSummary string `json:"lastSummary,omitempty"`
}

// Session is one agent session across every surface.
type Session struct {
	ID             string       `json:"id"`
	UserID         string       `json:"userID,omitempty"`
	Email          string       `json:"email,omitempty"`
	Agent          string       `json:"agent,omitempty"`
	Actor          Actor        `json:"actor"`
	Title          string       `json:"title,omitempty"`
	StartedAt      time.Time    `json:"startedAt"`
	LastActivityAt time.Time    `json:"lastActivityAt"`
	EndedAt        *time.Time   `json:"endedAt,omitempty"`
	State          SessionState `json:"state"`
}

// Warrant is intent-bound, just-in-time authority for a session: the task the
// user declared, the capability scopes that task needs, and a TTL. Drift from
// the warrant is a risk signal and a policy input, not an automatic block.
type Warrant struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionID"`
	// Task is the user's declared intent ("fix the flaky test in obot-platform/obot").
	Task string `json:"task"`
	// Scopes are the capability scopes the task is allowed: "mcp:github:*",
	// "mcp:github:create_pull_request", "egress:api.github.com", "shell",
	// "file:/Users/x/projects/obot/**", "model:*".
	Scopes []string `json:"scopes"`
	// Delegation is the chain this warrant was attenuated through, outermost
	// first. A sub-agent's warrant can only be narrower than its parent's.
	Delegation []string  `json:"delegation,omitempty"`
	IssuedAt   time.Time `json:"issuedAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	// Drifted reports that an action outside the scopes has been attempted.
	Drifted bool `json:"drifted,omitempty"`
	// DriftSummary describes the first drift seen.
	DriftSummary string `json:"driftSummary,omitempty"`
}

// LinkKey returns the stitching key for the session's client identifier, or
// "" when there is none.
func (k SessionKey) LinkKey() string {
	if k.ClientSessionID == "" {
		return ""
	}
	return "client:" + k.Agent + ":" + k.ClientSessionID
}

// FallbackKey returns the stitching key used when no client identifier is
// known. It is the actor: a device and agent, an environment, or a user, API
// key and agent.
func (k SessionKey) FallbackKey() string {
	switch {
	case k.EnvironmentID != "":
		return "env:" + k.EnvironmentID
	case k.DeviceID != "":
		return "device:" + k.DeviceID + ":" + k.Agent
	default:
		return "user:" + k.UserID + ":key:" + k.APIKeyID + ":" + k.Agent
	}
}

// Active reports whether the warrant is in force at t.
func (w *Warrant) Active(t time.Time) bool {
	return w != nil && !t.Before(w.IssuedAt) && t.Before(w.ExpiresAt)
}

// Covers reports whether an action is within the warrant's scopes.
func (w *Warrant) Covers(a Action) bool {
	if w == nil {
		return true
	}
	for _, scope := range w.Scopes {
		if scopeCovers(scope, a) {
			return true
		}
	}
	return false
}

func scopeCovers(scope string, a Action) bool {
	kind, rest, _ := strings.Cut(scope, ":")
	switch kind {
	case "*":
		return true
	case "mcp":
		if a.Kind != KindToolCall {
			return false
		}
		server, tool, _ := strings.Cut(rest, ":")
		if tool == "" {
			tool = "*"
		}
		return globMatch(server, a.Server) && globMatch(tool, a.Tool)
	case "shell":
		return a.Kind == KindShellExec
	case "egress":
		return a.Kind == KindNetEgress && globMatch(rest, a.Host)
	case "file":
		return (a.Kind == KindFileRead || a.Kind == KindFileWrite) && globMatch(rest, a.File)
	case "model":
		return a.Kind == KindLLMTurn && globMatch(rest, a.Model)
	}
	return false
}

// globMatch supports "*" anywhere and a "**" suffix for paths. It is
// case-insensitive.
func globMatch(pattern, value string) bool {
	pattern, value = strings.ToLower(pattern), strings.ToLower(value)
	if pattern == "*" || pattern == "**" || pattern == value {
		return true
	}
	if strings.HasSuffix(pattern, "/**") {
		return strings.HasPrefix(value, strings.TrimSuffix(pattern, "**"))
	}
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return false
	}
	if !strings.HasPrefix(value, parts[0]) {
		return false
	}
	value = value[len(parts[0]):]
	for i := 1; i < len(parts); i++ {
		part := parts[i]
		if part == "" {
			if i == len(parts)-1 {
				return true
			}
			continue
		}
		idx := strings.Index(value, part)
		if idx < 0 {
			return false
		}
		if i == len(parts)-1 {
			return strings.HasSuffix(value, part)
		}
		value = value[idx+len(part):]
	}
	return value == ""
}

// NewID returns a random identifier with a prefix.
func NewID(prefix string) string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return prefix + "-" + hex.EncodeToString(b[:])
}

// NewSession creates a session for a subject.
func NewSession(subject Subject, key SessionKey, now time.Time) *Session {
	s := &Session{
		ID:             NewID("ses"),
		UserID:         subject.UserID,
		Email:          subject.Email,
		Agent:          key.Agent,
		Actor:          subject.Actor,
		StartedAt:      now,
		LastActivityAt: now,
	}
	if s.Agent == "" {
		s.Agent = subject.Actor.Agent
	}
	s.Actor.ClientSessionID = key.ClientSessionID
	return s
}

// Touch records activity on the session.
func (s *Session) Touch(now time.Time, surface Surface) {
	if now.After(s.LastActivityAt) {
		s.LastActivityAt = now
	}
	if !slices.Contains(s.State.Surfaces, surface) {
		s.State.Surfaces = append(s.State.Surfaces, surface)
	}
}

// Idle reports whether the session has been quiet long enough that new
// activity should start a new session.
func (s *Session) Idle(now time.Time) bool {
	return s.EndedAt != nil || now.Sub(s.LastActivityAt) > SessionIdleTimeout
}

// AddLabels adds labels to the session's taint, remembering their origin.
func (st *SessionState) AddLabels(labels Labels, origin LabelOrigin) (added Labels) {
	for _, label := range labels.Normalize() {
		if !st.Labels.Has(label) {
			st.Labels = st.Labels.Add(label)
			added = added.Add(label)
		}
		if st.Origins == nil {
			st.Origins = map[Label][]LabelOrigin{}
		}
		if len(st.Origins[label]) < maxSessionLabelOrigins {
			st.Origins[label] = append(st.Origins[label], origin)
		}
	}
	return added
}

// AddFingerprints remembers fingerprints, dropping the oldest past the bound.
func (st *SessionState) AddFingerprints(fps []Fingerprint) {
	st.Fingerprints = append(st.Fingerprints, fps...)
	if over := len(st.Fingerprints) - maxSessionFingerprints; over > 0 {
		st.Fingerprints = slices.Clone(st.Fingerprints[over:])
	}
}

// Recompute derives the risk score from the state. It is additive and bounded:
// the score explains itself through the labels and counters that produced it.
func (st *SessionState) Recompute() {
	risk := 0
	if st.Labels.Has(LabelUntrusted) {
		risk += 30
	}
	if st.Labels.Confidential() {
		risk += 15
	}
	if st.Labels.Has(LabelSecret) {
		risk += 15
	}
	if st.Labels.Has(LabelHoneytoken) {
		risk += 40
	}
	if st.ExternalOut > 0 {
		risk += 10
	}
	risk += min(st.Denied*5, 20)
	if st.Warrant != nil && st.Warrant.Drifted {
		risk += 10
	}
	st.Risk = min(risk, 100)
}

// Trifecta reports whether the session holds both untrusted content and
// confidential data, so any external write completes the lethal trifecta.
func (st *SessionState) Trifecta() bool {
	return st.Labels.Has(LabelUntrusted) && st.Labels.Confidential()
}

// Explain returns a short, content-free sentence describing the session's
// taint: "read 2 untrusted sources (github.search_issues, web.fetch) and
// confidential data (salesforce.query)".
func (st *SessionState) Explain() string {
	var parts []string
	describe := func(label Label, what string) {
		origins := st.Origins[label]
		if len(origins) == 0 {
			if st.Labels.Has(label) {
				parts = append(parts, what)
			}
			return
		}
		var names []string
		for _, o := range origins {
			if o.Summary != "" && !slices.Contains(names, o.Summary) {
				names = append(names, o.Summary)
			}
		}
		if len(names) > 3 {
			names = append(names[:3], "...")
		}
		if len(names) == 0 {
			parts = append(parts, what)
			return
		}
		parts = append(parts, what+" ("+strings.Join(names, ", ")+")")
	}
	describe(LabelUntrusted, "untrusted content")
	for _, label := range ConfidentialLabels {
		if st.Labels.Has(label) {
			describe(label, string(label)+" data")
		}
	}
	if st.Labels.Has(LabelHoneytoken) {
		describe(LabelHoneytoken, "a honeytoken")
	}
	if len(parts) == 0 {
		return "the session has read nothing sensitive"
	}
	return "the session has read " + strings.Join(parts, " and ")
}
