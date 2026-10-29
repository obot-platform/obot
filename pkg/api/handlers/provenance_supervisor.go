package handlers

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/provenance"
	"github.com/obot-platform/obot/pkg/provenance/redteam"
)

const (
	// supervisorWindow is the window status and insights look back over.
	supervisorWindow = 24 * time.Hour
	// supervisorInsightsTTL is how long computed insights are reused.
	supervisorInsightsTTL = 30 * time.Second
	// supervisorApprovalAging is how long a pending approval may wait before
	// it is an insight.
	supervisorApprovalAging = 10 * time.Minute
	// supervisorAskTimeout bounds an answer; supervisorBriefTimeout bounds the
	// briefing, which runs while insights are computed.
	supervisorAskTimeout   = 60 * time.Second
	supervisorBriefTimeout = 20 * time.Second
	// supervisorAskEvents and supervisorAskSessionEvents bound the event lines
	// an answer sees.
	supervisorAskEvents        = 40
	supervisorAskSessionEvents = 60
	supervisorMaxQuestion      = 2000
	// supervisorReasonChars bounds a decision reason quoted in an answer's
	// context.
	supervisorReasonChars = 300
	// supervisorRiskyRisk is the risk at which an open session is an insight.
	supervisorRiskyRisk = 60
	// supervisorCriticalRisk makes a risky session critical.
	supervisorCriticalRisk = 80
	// supervisorItemsPerKind bounds the insights of one kind.
	supervisorItemsPerKind = 5
	// supervisorApprovalScan bounds the approvals scanned.
	supervisorApprovalScan = 1000

	insightSeverityCritical = "critical"
	insightSeverityWarning  = "warning"
	insightSeverityInfo     = "info"

	insightShadowPolicy      = "shadow-policy"
	insightOpenRiskySession  = "open-risky-session"
	insightHoneytokenTripped = "honeytoken-tripped"
	insightApprovalsAging    = "approvals-aging"
	insightCoverageGap       = "coverage-gap"
	insightUnlabeledTools    = "unlabeled-tools"

	supervisorNotConfigured = "the Supervisor is not configured"
)

// supervisorBriefer is implemented by a Supervisor that can write a short
// first-person briefing from content-free facts.
type supervisorBriefer interface {
	Brief(ctx context.Context, facts string) (text, model string, err error)
}

// supervisorAsker is implemented by a Supervisor that can answer an
// administrator's question from a content-free context.
type supervisorAsker interface {
	Ask(ctx context.Context, question, context string) (answer, model string, err error)
}

// supervisorModeler is implemented by a Supervisor that can report the text
// and decision models it uses.
type supervisorModeler interface {
	Models(ctx context.Context) (text, decision string)
}

// SupervisorStatus is the Supervisor's presence: whether it is on, what it is
// watching and what it decided in the last 24 hours.
type SupervisorStatus struct {
	Enabled         bool                     `json:"enabled"`
	Configured      bool                     `json:"configured"`
	Model           string                   `json:"model"`
	DeciderModel    string                   `json:"deciderModel"`
	Watching        SupervisorWatching       `json:"watching"`
	LastDecisionAt  *time.Time               `json:"lastDecisionAt"`
	LastJudgementAt *time.Time               `json:"lastJudgementAt"`
	Today           SupervisorDecisionCounts `json:"today"`
	// PendingApprovals counts every pending approval, not only today's.
	PendingApprovals int `json:"pendingApprovals"`
}

// SupervisorWatching is what the Supervisor saw in the window.
type SupervisorWatching struct {
	Sessions     int                  `json:"sessions"`
	OpenSessions int                  `json:"openSessions"`
	Surfaces     []provenance.Surface `json:"surfaces"`
}

// SupervisorDecisionCounts counts the window's decisions. Allowed, denied and
// held are enforced outcomes; shadowed counts decisions a shadow policy
// decided; judged counts decisions the Supervisor made; triaged counts
// approval requests it reviewed.
type SupervisorDecisionCounts struct {
	Decided  int `json:"decided"`
	Judged   int `json:"judged"`
	Held     int `json:"held"`
	Denied   int `json:"denied"`
	Allowed  int `json:"allowed"`
	Shadowed int `json:"shadowed"`
	Triaged  int `json:"triaged"`
}

// SupervisorInsights is what the Supervisor noticed, with a briefing.
type SupervisorInsights struct {
	GeneratedAt   time.Time           `json:"generatedAt"`
	Briefing      string              `json:"briefing"`
	BriefingModel string              `json:"briefingModel"`
	Items         []SupervisorInsight `json:"items"`
}

// SupervisorInsight is one finding, optionally with a one-click action that
// calls an existing provenance endpoint.
type SupervisorInsight struct {
	ID        string                   `json:"id"`
	Kind      string                   `json:"kind"`
	Severity  string                   `json:"severity"`
	Title     string                   `json:"title"`
	Detail    string                   `json:"detail"`
	SessionID string                   `json:"sessionID,omitempty"`
	PolicyID  string                   `json:"policyID,omitempty"`
	Action    *SupervisorInsightAction `json:"action,omitempty"`
}

// SupervisorInsightAction is a request the UI can make to act on an insight.
type SupervisorInsightAction struct {
	Label  string `json:"label"`
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   any    `json:"body,omitempty"`
}

type supervisorAskRequest struct {
	Question  string `json:"question"`
	SessionID string `json:"sessionID"`
}

type supervisorAskResponse struct {
	Answer  string `json:"answer"`
	Model   string `json:"model"`
	Context struct {
		Sessions int `json:"sessions"`
		Events   int `json:"events"`
	} `json:"context"`
}

// supervisorInsightsCache holds the last computed insights. Its zero value is
// ready to use.
type supervisorInsightsCache struct {
	lock  sync.Mutex
	at    time.Time
	value *SupervisorInsights
}

// supervisorSnapshot is everything status, insights and answers are computed
// from: the window's sessions and decisions plus the reference data.
type supervisorSnapshot struct {
	now, since  time.Time
	settings    provenance.Settings
	supervisor  provenance.Supervisor
	sessions    []provenance.Session
	events      []provenance.Event // decided, newest first
	pending     []provenance.Approval
	approvals   []provenance.Approval // created in the window
	policies    []provenance.Policy
	honeytokens []provenance.Honeytoken
	toolLabels  []provenance.ToolLabel
	kills       []provenance.Kill
	lastDecided *time.Time
}

// ---- endpoints ----

// SupervisorStatus reports the Supervisor's presence and today's counts.
func (h *ProvenanceHandler) SupervisorStatus(req api.Context) error {
	snap, err := h.supervisorSnapshot(req.Context())
	if err != nil {
		return err
	}
	return req.Write(supervisorStatusOf(req.Context(), snap))
}

// SupervisorInsights reports what the Supervisor noticed in the last 24 hours
// with a first-person briefing. Results are cached for 30 seconds;
// ?refresh=true recomputes them.
func (h *ProvenanceHandler) SupervisorInsights(req api.Context) error {
	refresh := req.URL.Query().Get("refresh") == "true"
	h.insights.lock.Lock()
	defer h.insights.lock.Unlock()
	if !refresh && h.insights.value != nil && time.Since(h.insights.at) < supervisorInsightsTTL {
		return req.Write(h.insights.value)
	}
	ctx := req.Context()
	snap, err := h.supervisorSnapshot(ctx)
	if err != nil {
		return err
	}
	status := supervisorStatusOf(ctx, snap)
	out := &SupervisorInsights{GeneratedAt: snap.now, Items: supervisorInsightItems(snap)}
	out.Briefing, out.BriefingModel = supervisorBriefing(ctx, snap, status, out.Items)
	h.insights.at, h.insights.value = time.Now(), out
	return req.Write(out)
}

// SupervisorAsk answers an administrator's question from a content-free
// context: status, insights, recent decisions and, when a session is named,
// that session's labels, warrant and events.
func (h *ProvenanceHandler) SupervisorAsk(req api.Context) error {
	var in supervisorAskRequest
	if err := req.Read(&in); err != nil {
		return types.NewErrBadRequest("invalid request: %v", err)
	}
	in.Question = strings.TrimSpace(in.Question)
	if in.Question == "" {
		return types.NewErrBadRequest("question is required")
	}
	if len(in.Question) > supervisorMaxQuestion {
		return types.NewErrBadRequest("question must be at most %d characters", supervisorMaxQuestion)
	}
	ctx := req.Context()
	asker, ok := h.engine.Supervisor().(supervisorAsker)
	if !ok || !h.engine.Settings(ctx).SupervisorEnabled {
		return req.WriteCode(map[string]string{"error": supervisorNotConfigured}, http.StatusServiceUnavailable)
	}
	var session *provenance.Session
	if id := strings.TrimSpace(in.SessionID); id != "" {
		s, err := h.engine.Store().GetSession(ctx, id)
		if err != nil {
			return provenanceLookupErr(err, "session", id)
		}
		session = s
	}
	snap, err := h.supervisorSnapshot(ctx)
	if err != nil {
		return err
	}
	var sessionEvents []provenance.Event
	if session != nil {
		if sessionEvents, _, err = h.engine.Store().ListEvents(ctx, provenance.EventQuery{SessionID: session.ID, Limit: supervisorAskSessionEvents}); err != nil {
			return err
		}
	}
	text, sessions, events := supervisorAskContext(ctx, snap, session, sessionEvents)

	actx, cancel := context.WithTimeout(ctx, supervisorAskTimeout)
	defer cancel()
	answer, model, err := asker.Ask(actx, in.Question, text)
	if err != nil {
		return types.NewErrHTTP(http.StatusBadGateway, "the Supervisor could not answer: "+err.Error())
	}
	var out supervisorAskResponse
	out.Answer, out.Model = answer, model
	out.Context.Sessions, out.Context.Events = sessions, events
	return req.Write(out)
}

// ---- snapshot and status ----

func (h *ProvenanceHandler) supervisorSnapshot(ctx context.Context) (*supervisorSnapshot, error) {
	store := h.engine.Store()
	now := time.Now().UTC()
	snap := &supervisorSnapshot{
		now:        now,
		since:      now.Add(-supervisorWindow),
		settings:   h.engine.Settings(ctx),
		supervisor: h.engine.Supervisor(),
	}
	var err error
	if snap.sessions, _, err = store.ListSessions(ctx, provenance.SessionQuery{Start: snap.since, Limit: provenanceOverviewSessions, SortDesc: true}); err != nil {
		return nil, err
	}

	decided := true
	for offset := 0; offset < provenanceOverviewEvents; {
		events, _, err := store.ListEvents(ctx, provenance.EventQuery{Decided: &decided, Start: snap.since, Limit: provenanceOverviewBatch, Offset: offset})
		if err != nil {
			return nil, err
		}
		if len(events) == 0 {
			break
		}
		offset += len(events)
		for _, ev := range events {
			if ev.Decision != nil {
				snap.events = append(snap.events, ev)
			}
		}
	}
	// Newest first by time, whatever order the store returned them in.
	slices.SortStableFunc(snap.events, func(a, b provenance.Event) int { return b.At.Compare(a.At) })
	if len(snap.events) > 0 {
		at := snap.events[0].At
		snap.lastDecided = &at
	} else {
		last, _, err := store.ListEvents(ctx, provenance.EventQuery{Decided: &decided, Limit: 1})
		if err != nil {
			return nil, err
		}
		if len(last) > 0 {
			at := last[0].At
			snap.lastDecided = &at
		}
	}

	if snap.pending, _, err = store.ListApprovals(ctx, provenance.ApprovalQuery{Status: provenance.ApprovalPending, Limit: supervisorApprovalScan}); err != nil {
		return nil, err
	}
	approvals, _, err := store.ListApprovals(ctx, provenance.ApprovalQuery{Limit: supervisorApprovalScan})
	if err != nil {
		return nil, err
	}
	for _, a := range approvals {
		if !a.CreatedAt.Before(snap.since) {
			snap.approvals = append(snap.approvals, a)
		}
	}
	if snap.policies, err = store.ListPolicies(ctx); err != nil {
		return nil, err
	}
	if snap.honeytokens, err = store.ListHoneytokens(ctx); err != nil {
		return nil, err
	}
	if snap.toolLabels, err = store.ListToolLabels(ctx); err != nil {
		return nil, err
	}
	if snap.kills, err = store.ListKills(ctx); err != nil {
		return nil, err
	}
	return snap, nil
}

// judgedBySupervisor reports whether the Supervisor made the decision: a
// judge-effect policy fired in enforce mode, or the reason carries the
// Supervisor's rationale.
func judgedBySupervisor(d *provenance.Decision) bool {
	if d == nil {
		return false
	}
	if strings.Contains(d.Reason, "Supervisor") {
		return true
	}
	return slices.ContainsFunc(d.Matched, func(m provenance.PolicyMatch) bool {
		return m.Effect == provenance.EffectJudge && !m.Shadow
	})
}

// active reports whether a session is still running: not ended, not idle and
// not stopped by a kill switch.
func (snap *supervisorSnapshot) active(s provenance.Session) bool {
	if s.EndedAt != nil || s.State.Killed || s.Idle(snap.now) {
		return false
	}
	return !slices.ContainsFunc(snap.kills, func(k provenance.Kill) bool { return k.Matches(&s, snap.now) })
}

func supervisorStatusOf(ctx context.Context, snap *supervisorSnapshot) SupervisorStatus {
	out := SupervisorStatus{
		Configured:       snap.supervisor != nil,
		LastDecisionAt:   snap.lastDecided,
		PendingApprovals: len(snap.pending),
	}
	out.Enabled = out.Configured && snap.settings.SupervisorEnabled
	out.Model, out.DeciderModel = supervisorModelNames(ctx, snap)

	seen := map[provenance.Surface]bool{}
	for _, s := range snap.sessions {
		out.Watching.Sessions++
		if snap.active(s) {
			out.Watching.OpenSessions++
		}
		for _, sf := range s.State.Surfaces {
			seen[sf] = true
		}
	}
	out.Watching.Surfaces = []provenance.Surface{}
	for _, sf := range provenance.AllSurfaces {
		if seen[sf] {
			out.Watching.Surfaces = append(out.Watching.Surfaces, sf)
		}
	}

	for _, ev := range snap.events {
		d := ev.Decision
		out.Today.Decided++
		held := !d.Shadow && d.Effect == provenance.EffectRequireApproval
		switch {
		case d.Shadow:
			out.Today.Shadowed++
		case held:
			out.Today.Held++
		case !d.Allow:
			out.Today.Denied++
		default:
			out.Today.Allowed++
		}
		if judgedBySupervisor(d) {
			out.Today.Judged++
			if out.LastJudgementAt == nil {
				at := ev.At
				out.LastJudgementAt = &at
			}
		}
	}
	for _, a := range snap.approvals {
		if a.Triage != nil {
			out.Today.Triaged++
		}
	}
	return out
}

// supervisorModelNames names the Supervisor's text and decision models: the
// model that last answered when the Supervisor reports one, otherwise the
// configured model, otherwise the default aliases. An answering model is a
// readable gateway model name, where the configured one may be a Model ID.
func supervisorModelNames(ctx context.Context, snap *supervisorSnapshot) (text, decision string) {
	configured := snap.settings.SupervisorModel
	text = cmp.Or(configured, string(types.DefaultModelAliasTypeLLM))
	decision = cmp.Or(configured, string(types.DefaultModelAliasTypeLLMMini))
	if m, ok := snap.supervisor.(supervisorModeler); ok {
		t, d := m.Models(ctx)
		text, decision = cmp.Or(t, text), cmp.Or(d, decision)
	}
	if decision == configured || decision == string(types.DefaultModelAliasTypeLLMMini) {
		// A triage records the model that answered, which names the decision
		// model without a lookup.
		for _, a := range snap.approvals {
			if a.Triage != nil && a.Triage.Model != "" {
				return text, a.Triage.Model
			}
		}
	}
	return text, decision
}

// ---- insights ----

func supervisorInsightItems(all *supervisorSnapshot) []SupervisorInsight {
	snap := withoutRedTeam(all)
	var items []SupervisorInsight
	items = append(items, honeytokenInsights(snap)...)
	items = append(items, riskySessionInsights(snap)...)
	items = append(items, agingApprovalInsights(snap)...)
	items = append(items, shadowPolicyInsights(snap)...)
	items = append(items, coverageInsights(snap)...)
	items = append(items, unlabeledToolInsights(snap)...)
	slices.SortStableFunc(items, func(a, b SupervisorInsight) int {
		return severityRank(a.Severity) - severityRank(b.Severity)
	})
	if items == nil {
		items = []SupervisorInsight{}
	}
	return items
}

// withoutRedTeam returns the snapshot without the red-team harness's
// synthetic sessions, decisions and approvals: they are assurance runs, not
// findings an administrator should act on.
func withoutRedTeam(snap *supervisorSnapshot) *supervisorSnapshot {
	out := *snap
	redTeam := map[string]bool{}
	out.sessions = nil
	for _, s := range snap.sessions {
		if strings.HasPrefix(s.UserID, redteam.Prefix) {
			redTeam[s.ID] = true
			continue
		}
		out.sessions = append(out.sessions, s)
	}
	out.events = nil
	for _, ev := range snap.events {
		if !redTeam[ev.SessionID] && !strings.HasPrefix(ev.UserID, redteam.Prefix) {
			out.events = append(out.events, ev)
		}
	}
	out.pending = nil
	for _, a := range snap.pending {
		if !redTeam[a.SessionID] && !strings.HasPrefix(a.UserID, redteam.Prefix) {
			out.pending = append(out.pending, a)
		}
	}
	return &out
}

func severityRank(s string) int {
	switch s {
	case insightSeverityCritical:
		return 0
	case insightSeverityWarning:
		return 1
	}
	return 2
}

func shadowPolicyInsights(snap *supervisorSnapshot) []SupervisorInsight {
	counts := map[string]int{}
	sessions := map[string]map[string]bool{}
	for _, ev := range snap.events {
		seen := map[string]bool{}
		for _, m := range ev.Decision.Matched {
			if !m.Shadow || seen[m.PolicyID] {
				continue
			}
			seen[m.PolicyID] = true
			counts[m.PolicyID]++
			if sessions[m.PolicyID] == nil {
				sessions[m.PolicyID] = map[string]bool{}
			}
			sessions[m.PolicyID][ev.SessionID] = true
		}
	}
	var candidates []provenance.Policy
	for _, p := range snap.policies {
		if p.Enabled && p.Mode == provenance.ModeShadow && counts[p.ID] > 0 {
			candidates = append(candidates, p)
		}
	}
	slices.SortStableFunc(candidates, func(a, b provenance.Policy) int { return counts[b.ID] - counts[a.ID] })
	var out []SupervisorInsight
	for _, p := range candidates[:min(len(candidates), supervisorItemsPerKind)] {
		n, nSessions := counts[p.ID], len(sessions[p.ID])
		severity := insightSeverityInfo
		if p.Effect.Blocks() || p.Effect == provenance.EffectJudge {
			severity = insightSeverityWarning
		}
		enforced := p
		enforced.Mode = provenance.ModeEnforce
		out = append(out, SupervisorInsight{
			ID:       insightShadowPolicy + ":" + p.ID,
			Kind:     insightShadowPolicy,
			Severity: severity,
			Title:    fmt.Sprintf("%q would have %s", p.Name, wouldHave(p.Effect, plural(n, "action", "actions"))),
			Detail: fmt.Sprintf("It is in shadow mode, so I let those through and recorded what it would have done: %s in %s over the last 24 hours. "+
				"If those look right, enforce it.", plural(n, "decision", "decisions"), plural(nSessions, "session", "sessions")),
			PolicyID: p.ID,
			Action: &SupervisorInsightAction{
				Label:  "Enforce policy",
				Method: http.MethodPut,
				Path:   "/api/provenance/policies/" + p.ID,
				Body:   enforced,
			},
		})
	}
	return out
}

func riskySessionInsights(snap *supervisorSnapshot) []SupervisorInsight {
	var risky []provenance.Session
	for _, s := range snap.sessions {
		if snap.active(s) && (s.State.Trifecta() || s.State.Risk >= supervisorRiskyRisk) {
			risky = append(risky, s)
		}
	}
	slices.SortStableFunc(risky, func(a, b provenance.Session) int { return b.State.Risk - a.State.Risk })
	var out []SupervisorInsight
	for _, s := range risky[:min(len(risky), supervisorItemsPerKind)] {
		severity := insightSeverityWarning
		if s.State.Risk >= supervisorCriticalRisk || s.State.Labels.Has(provenance.LabelHoneytoken) {
			severity = insightSeverityCritical
		}
		who := sessionOwner(s)
		title := fmt.Sprintf("%s is open at risk %d", who, s.State.Risk)
		if s.State.Trifecta() {
			title = who + " holds untrusted content and confidential data"
		}
		detail := upperFirst(s.State.Explain()) + fmt.Sprintf("; risk %d", s.State.Risk)
		if s.State.LastSummary != "" {
			detail += fmt.Sprintf("; last activity %s: %s", s.LastActivityAt.UTC().Format(time.RFC3339), s.State.LastSummary)
		}
		detail += "."
		if s.State.Trifecta() {
			detail += " Any external write from here completes the lethal trifecta. I'm watching it; stopping it denies everything it tries next."
		} else {
			detail += " I'm watching it; stopping it denies everything it tries next."
		}
		out = append(out, SupervisorInsight{
			ID:        insightOpenRiskySession + ":" + s.ID,
			Kind:      insightOpenRiskySession,
			Severity:  severity,
			Title:     title,
			Detail:    detail,
			SessionID: s.ID,
			Action:    killSessionAction(s.ID, "stopped from the Supervisor's insight: "+title),
		})
	}
	return out
}

func honeytokenInsights(snap *supervisorSnapshot) []SupervisorInsight {
	var tripped []provenance.Honeytoken
	for _, t := range snap.honeytokens {
		if t.LastTrip != nil && !t.LastTrip.Before(snap.since) {
			tripped = append(tripped, t)
		}
	}
	// The session that tripped a token is only known for certain when one
	// token tripped.
	var session *provenance.Session
	if len(tripped) == 1 {
		for _, ev := range snap.events {
			if ev.Decision.Honeytoken {
				for i := range snap.sessions {
					if snap.sessions[i].ID == ev.SessionID {
						session = &snap.sessions[i]
						break
					}
				}
				break
			}
		}
	}
	var out []SupervisorInsight
	for _, t := range tripped {
		item := SupervisorInsight{
			ID:       insightHoneytokenTripped + ":" + t.ID,
			Kind:     insightHoneytokenTripped,
			Severity: insightSeverityCritical,
			Title:    fmt.Sprintf("Honeytoken %q was tripped", t.Name),
			Detail: fmt.Sprintf("A planted %s canary showed up in an outbound action %s, most recently at %s. That is definitive exfiltration evidence.",
				t.Kind, plural(t.Tripped, "time", "times"), t.LastTrip.UTC().Format(time.RFC3339)),
		}
		if session != nil {
			item.SessionID = session.ID
			item.Detail += " I traced it to " + sessionOwner(*session) + "."
			if snap.active(*session) {
				item.Action = killSessionAction(session.ID, "stopped from the Supervisor's insight: "+item.Title)
			}
		}
		out = append(out, item)
	}
	return out
}

func agingApprovalInsights(snap *supervisorSnapshot) []SupervisorInsight {
	var aging []provenance.Approval
	for _, a := range snap.pending {
		if snap.now.Sub(a.CreatedAt) > supervisorApprovalAging && snap.now.Before(a.ExpiresAt) {
			aging = append(aging, a)
		}
	}
	slices.SortStableFunc(aging, func(a, b provenance.Approval) int { return a.CreatedAt.Compare(b.CreatedAt) })
	var out []SupervisorInsight
	for _, a := range aging[:min(len(aging), supervisorItemsPerKind)] {
		wait := humanDuration(snap.now.Sub(a.CreatedAt))
		item := SupervisorInsight{
			ID:        insightApprovalsAging + ":" + a.ID,
			Kind:      insightApprovalsAging,
			Severity:  insightSeverityWarning,
			Title:     fmt.Sprintf("%s has waited %s for approval", provenance.ActionSummary(a.Action), wait),
			Detail:    fmt.Sprintf("%q held it; it expires at %s.", a.PolicyName, a.ExpiresAt.UTC().Format(time.RFC3339)),
			SessionID: a.SessionID,
			PolicyID:  a.PolicyID,
		}
		if t := a.Triage; t != nil {
			item.Detail += fmt.Sprintf(" My recommendation: %s (%.0f%% confident). %s", t.Recommendation, t.Confidence*100, t.Rationale)
			switch t.Recommendation {
			case "approve", "deny":
				item.Action = &SupervisorInsightAction{
					Label:  "Accept my recommendation: " + t.Recommendation,
					Method: http.MethodPost,
					Path:   "/api/provenance/approvals/" + a.ID + "/" + t.Recommendation,
					Body:   approvalResolveRequest{Note: "accepted the Supervisor's recommendation"},
				}
			}
		} else {
			item.Detail += " I haven't triaged it."
		}
		out = append(out, item)
	}
	return out
}

func coverageInsights(snap *supervisorSnapshot) []SupervisorInsight {
	if len(snap.sessions) == 0 {
		return []SupervisorInsight{{
			ID:       insightCoverageGap,
			Kind:     insightCoverageGap,
			Severity: insightSeverityInfo,
			Title:    "I haven't seen any agent sessions in the last 24 hours",
			Detail:   "Nothing reached me from the LLM gateway, the MCP gateway, device hooks, sandbox egress or Copilot Studio. Once agents use them, I'll watch here.",
		}}
	}
	coverage := provenance.CoverageOf(snap.sessions)
	var missing []string
	for _, sf := range provenance.AllSurfaces {
		if coverage.Surfaces[sf] == 0 {
			missing = append(missing, string(sf))
		}
	}
	if len(missing) == 0 && coverage.Blind == 0 {
		return nil
	}
	severity := insightSeverityInfo
	if coverage.Blind*2 > len(snap.sessions) {
		severity = insightSeverityWarning
	}
	var title string
	var detail []string
	if len(missing) > 0 {
		title = fmt.Sprintf("I can't see the %s %s", joinAnd(missing), pluralWord(len(missing), "surface", "surfaces"))
		detail = append(detail, fmt.Sprintf("No sessions reported from %s in the last 24 hours, so what agents do there is invisible to me.", joinAnd(missing)))
	}
	if coverage.Blind > 0 {
		if title == "" {
			title = fmt.Sprintf("%s were seen on only one surface", plural(coverage.Blind, "session", "sessions"))
		}
		detail = append(detail, fmt.Sprintf("%d of %s were seen on only one surface, so I can't connect what they read with what they sent.",
			coverage.Blind, plural(len(snap.sessions), "session", "sessions")))
	}
	return []SupervisorInsight{{
		ID:       insightCoverageGap,
		Kind:     insightCoverageGap,
		Severity: severity,
		Title:    title,
		Detail:   strings.Join(detail, " "),
	}}
}

func unlabeledToolInsights(snap *supervisorSnapshot) []SupervisorInsight {
	used := map[string]bool{}
	for _, ev := range snap.events {
		if a := ev.Action; a != nil && a.Kind == provenance.KindToolCall && a.Tool != "" {
			used[a.Server+"\x00"+a.Tool] = true
		}
	}
	var names []string
	for _, l := range snap.toolLabels {
		if l.Source == "heuristic" && used[l.Server+"\x00"+l.Tool] {
			name := l.Tool
			if l.Server != "" && l.Server != "*" {
				name = l.Server + "." + l.Tool
			}
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	slices.Sort(names)
	shown := names[:min(len(names), supervisorItemsPerKind)]
	list := strings.Join(shown, ", ")
	if len(names) > len(shown) {
		list += fmt.Sprintf(" and %d more", len(names)-len(shown))
	}
	return []SupervisorInsight{{
		ID:       insightUnlabeledTools,
		Kind:     insightUnlabeledTools,
		Severity: insightSeverityInfo,
		Title:    fmt.Sprintf("%s used today %s on heuristic labels", plural(len(names), "tool", "tools"), pluralWord(len(names), "runs", "run")),
		Detail: fmt.Sprintf("I'm labelling %s from the name and description only. Confirm their capability labels so policies see what they really do.",
			list),
	}}
}

func killSessionAction(sessionID, reason string) *SupervisorInsightAction {
	return &SupervisorInsightAction{
		Label:  "Stop this session",
		Method: http.MethodPost,
		Path:   "/api/provenance/kills",
		Body:   killRequest{Scope: provenance.KillScopeSession, Target: sessionID, Reason: reason},
	}
}

// ---- briefing ----

// supervisorFacts renders the content-free facts a briefing is written from.
func supervisorFacts(status SupervisorStatus, items []SupervisorInsight) string {
	var b strings.Builder
	surfaces := make([]string, 0, len(status.Watching.Surfaces))
	for _, sf := range status.Watching.Surfaces {
		surfaces = append(surfaces, string(sf))
	}
	fmt.Fprintf(&b, "Window: the last 24 hours.\n")
	fmt.Fprintf(&b, "Watching: %d sessions, %d still open, on surfaces: %s.\n", status.Watching.Sessions, status.Watching.OpenSessions, cmp.Or(strings.Join(surfaces, ", "), "none"))
	t := status.Today
	fmt.Fprintf(&b, "Decisions: %d decided: %d allowed, %d denied, %d held for approval, %d recorded by shadow policies. I judged %d gray-area actions myself and triaged %d approval requests.\n",
		t.Decided, t.Allowed, t.Denied, t.Held, t.Shadowed, t.Judged, t.Triaged)
	fmt.Fprintf(&b, "Pending approvals: %d.\n", status.PendingApprovals)
	if len(items) == 0 {
		b.WriteString("Findings: none.\n")
		return b.String()
	}
	b.WriteString("Findings (most urgent first):\n")
	for _, item := range items {
		fmt.Fprintf(&b, "- [%s] %s. %s\n", item.Severity, item.Title, item.Detail)
	}
	return b.String()
}

// supervisorBriefing asks the Supervisor for a briefing and falls back to a
// deterministic one.
func supervisorBriefing(ctx context.Context, snap *supervisorSnapshot, status SupervisorStatus, items []SupervisorInsight) (string, string) {
	if briefer, ok := snap.supervisor.(supervisorBriefer); ok && snap.settings.SupervisorEnabled {
		bctx, cancel := context.WithTimeout(ctx, supervisorBriefTimeout)
		defer cancel()
		if text, model, err := briefer.Brief(bctx, supervisorFacts(status, items)); err == nil && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text), model
		}
	}
	return fallbackBriefing(status, items), ""
}

func fallbackBriefing(status SupervisorStatus, items []SupervisorInsight) string {
	if status.Watching.Sessions == 0 {
		return "I haven't seen any agent sessions in the last 24 hours. Once agents work through Obot's gateways, device hooks or sandbox egress, I'll watch them here."
	}
	t := status.Today
	var b strings.Builder
	fmt.Fprintf(&b, "In the last 24 hours I watched %s (%d still open) and decided %s: %d allowed, %d denied and %d held for approval.",
		plural(status.Watching.Sessions, "session", "sessions"), status.Watching.OpenSessions, plural(t.Decided, "action", "actions"), t.Allowed, t.Denied, t.Held)
	if t.Judged > 0 {
		fmt.Fprintf(&b, " I made the call myself on %s.", plural(t.Judged, "gray-area action", "gray-area actions"))
	}
	var urgent []SupervisorInsight
	for _, item := range items {
		if item.Severity != insightSeverityInfo {
			urgent = append(urgent, item)
		}
	}
	switch {
	case len(urgent) > 0:
		fmt.Fprintf(&b, " %s %s your attention; the most urgent: %s.", upperFirst(plural(len(urgent), "thing", "things")), pluralWord(len(urgent), "needs", "need"), urgent[0].Title)
	case status.PendingApprovals > 0:
		fmt.Fprintf(&b, " %s waiting for you.", upperFirst(plural(status.PendingApprovals, "approval is", "approvals are")))
	default:
		b.WriteString(" Nothing needs your attention right now.")
	}
	return b.String()
}

// ---- ask ----

// supervisorAskContext renders the content-free context an answer sees and
// counts the sessions and events in it.
func supervisorAskContext(ctx context.Context, snap *supervisorSnapshot, session *provenance.Session, sessionEvents []provenance.Event) (string, int, int) {
	status := supervisorStatusOf(ctx, snap)
	var b strings.Builder
	b.WriteString("My status:\n")
	b.WriteString(supervisorFacts(status, supervisorInsightItems(snap)))

	sessions := map[string]bool{}
	events := 0
	recent := snap.events[:min(len(snap.events), supervisorAskEvents)]
	if len(recent) > 0 {
		b.WriteString("\nRecent decisions (oldest first; time, surface, session, action, labels, effect, reason):\n")
		for _, ev := range slices.Backward(recent) {
			b.WriteString(askEventLine(ev, true))
			b.WriteString("\n")
			sessions[ev.SessionID] = true
			events++
		}
	}

	if session != nil {
		sessions[session.ID] = true
		st := session.State
		fmt.Fprintf(&b, "\nThe session the administrator is asking about: %s, %s.\n", session.ID, sessionOwner(*session))
		fmt.Fprintf(&b, "Started %s, last activity %s", session.StartedAt.UTC().Format(time.RFC3339), session.LastActivityAt.UTC().Format(time.RFC3339))
		switch {
		case session.EndedAt != nil:
			fmt.Fprintf(&b, ", ended %s", session.EndedAt.UTC().Format(time.RFC3339))
		case st.Killed:
			b.WriteString(", stopped with the kill switch")
		case snap.active(*session):
			b.WriteString(", still open")
		}
		if session.Title != "" {
			fmt.Fprintf(&b, "; title %q", session.Title)
		}
		fmt.Fprintf(&b, ".\nRisk %d; labels %s; %d events, %d tool calls, %d denied, %d shadowed, %d approvals, %d external writes.\n",
			st.Risk, labelsOrNone(st.Labels), st.Events, st.ToolCalls, st.Denied, st.Shadowed, st.Approvals, st.ExternalOut)
		if len(st.Surfaces) > 0 {
			surfaces := make([]string, 0, len(st.Surfaces))
			for _, sf := range st.Surfaces {
				surfaces = append(surfaces, string(sf))
			}
			fmt.Fprintf(&b, "Surfaces: %s.\n", strings.Join(surfaces, ", "))
		}
		b.WriteString(upperFirst(st.Explain()) + ".\n")
		if len(st.Origins) > 0 {
			b.WriteString("Where each label came from:\n")
			for _, label := range st.Labels {
				var origins []string
				for _, o := range st.Origins[label] {
					origins = append(origins, fmt.Sprintf("%s at %s", cmp.Or(o.Summary, o.EventID), o.At.UTC().Format(time.RFC3339)))
				}
				if len(origins) > 0 {
					fmt.Fprintf(&b, "- %s: %s\n", label, strings.Join(origins, "; "))
				}
			}
		}
		if w := st.Warrant; w != nil {
			fmt.Fprintf(&b, "Warrant: task %q, scopes [%s], expires %s", w.Task, strings.Join(w.Scopes, ", "), w.ExpiresAt.UTC().Format(time.RFC3339))
			if w.Drifted {
				fmt.Fprintf(&b, ", drifted at %s", w.DriftSummary)
			}
			b.WriteString(".\n")
		} else {
			b.WriteString("Warrant: none issued.\n")
		}
		if len(sessionEvents) > 0 {
			b.WriteString("Its events (oldest first; time, surface, action, labels, effect, reason):\n")
			for _, ev := range slices.Backward(sessionEvents) {
				b.WriteString(askEventLine(ev, false))
				b.WriteString("\n")
				events++
			}
		}
	}
	return b.String(), len(sessions), events
}

// askEventLine renders one event content-free: time, surface, summary,
// labels, effect and reason.
func askEventLine(ev provenance.Event, withSession bool) string {
	parts := []string{ev.At.UTC().Format(time.RFC3339), cmp.Or(string(ev.Surface), "-")}
	if withSession {
		parts = append(parts, "session "+ev.SessionID)
	}
	summary := ev.Summary
	if summary == "" && ev.Action != nil {
		summary = provenance.ActionSummary(*ev.Action)
	}
	parts = append(parts, cmp.Or(summary, string(ev.Kind)))
	if len(ev.Labels) > 0 {
		parts = append(parts, "labels "+ev.Labels.String())
	}
	if d := ev.Decision; d != nil {
		effect := string(d.Effect)
		switch {
		case d.Killed:
			effect = "denied (killed)"
		case d.Shadow:
			effect = "allowed; would " + effect + " (shadow)"
		case !d.Allow:
			effect += " (blocked)"
		}
		if judgedBySupervisor(d) {
			effect += ", judged by me"
		}
		parts = append(parts, effect)
		if d.Reason != "" {
			parts = append(parts, "reason: "+truncateText(d.Reason, supervisorReasonChars))
		}
	}
	return strings.Join(parts, " | ")
}

// ---- text helpers ----

func sessionOwner(s provenance.Session) string {
	who := s.Email
	switch {
	case who != "":
	case s.UserID != "":
		who = "user " + s.UserID
	default:
		who = "an unknown user"
	}
	if agent := cmp.Or(s.Agent, s.Actor.Agent); agent != "" {
		return fmt.Sprintf("%s's %s session", who, agent)
	}
	return who + "'s session"
}

// wouldHave says what an effect would have done to what: "denied 3 actions".
func wouldHave(e provenance.Effect, what string) string {
	switch e {
	case provenance.EffectDeny:
		return "denied " + what
	case provenance.EffectRequireApproval:
		return "held " + what + " for approval"
	case provenance.EffectJudge:
		return "sent " + what + " to me to judge"
	case provenance.EffectRedact:
		return "redacted " + what
	case provenance.EffectQuarantine:
		return "quarantined " + what
	case provenance.EffectDownshift:
		return "downshifted " + what
	case provenance.EffectLog:
		return "logged " + what
	case provenance.EffectAllow:
		return "allowed " + what
	}
	return string(e) + " " + what
}

// humanDuration renders a wait as "12 min" or "2 h 5 min".
func humanDuration(d time.Duration) string {
	minutes := int(d.Round(time.Minute) / time.Minute)
	if minutes < 60 {
		return fmt.Sprintf("%d min", minutes)
	}
	if minutes%60 == 0 {
		return fmt.Sprintf("%d h", minutes/60)
	}
	return fmt.Sprintf("%d h %d min", minutes/60, minutes%60)
}

func plural(n int, one, many string) string {
	return fmt.Sprintf("%d %s", n, pluralWord(n, one, many))
}

func pluralWord(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func joinAnd(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func labelsOrNone(l provenance.Labels) string {
	if len(l) == 0 {
		return "none"
	}
	return l.String()
}

func truncateText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut] + "…"
}
