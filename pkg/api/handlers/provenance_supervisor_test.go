package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/provenance"
)

// fakeSupervisorAssistant is a Supervisor that also briefs, answers and
// reports its models.
type fakeSupervisorAssistant struct {
	fakeProvAPISupervisor
	briefs      int
	lastFacts   string
	lastAsk     string
	lastContext string
	err         error
}

func (f *fakeSupervisorAssistant) Brief(_ context.Context, facts string) (string, string, error) {
	f.briefs++
	f.lastFacts = facts
	if f.err != nil {
		return "", "", f.err
	}
	return "I watched one session and held nothing.", "fake-text", nil
}

func (f *fakeSupervisorAssistant) Ask(_ context.Context, question, facts string) (string, string, error) {
	f.lastAsk, f.lastContext = question, facts
	if f.err != nil {
		return "", "", f.err
	}
	return "I stopped nothing today.", "fake-text", nil
}

func (f *fakeSupervisorAssistant) Models(context.Context) (string, string) {
	return "fake-text", "fake-decider"
}

func provAPIInsight(t *testing.T, items []SupervisorInsight, kind string) SupervisorInsight {
	t.Helper()
	for _, item := range items {
		if item.Kind == kind {
			return item
		}
	}
	t.Fatalf("no %s insight in %+v", kind, items)
	return SupervisorInsight{}
}

func TestSupervisorStatus(t *testing.T) {
	h, e := newProvAPITest(t)

	rec := provAPIOK(t, h.SupervisorStatus, http.MethodGet, "/api/provenance/supervisor/status", nil, nil, nil)
	for _, field := range []string{`"lastDecisionAt":null`, `"lastJudgementAt":null`, `"surfaces":[]`} {
		if !strings.Contains(rec.Body.String(), field) {
			t.Fatalf("an empty status should have %s: %s", field, rec.Body.String())
		}
	}
	var status SupervisorStatus
	provAPIOK(t, h.SupervisorStatus, http.MethodGet, "/api/provenance/supervisor/status", nil, nil, &status)
	if status.Configured || status.Enabled || status.Model != "llm" || status.DeciderModel != "llm-mini" {
		t.Fatalf("without a Supervisor: %+v", status)
	}

	s := provAPISeedTrifecta(t, e)
	e.SetSupervisor(&fakeSupervisorAssistant{})
	provAPIOK(t, h.SupervisorStatus, http.MethodGet, "/api/provenance/supervisor/status", nil, nil, &status)
	if !status.Configured || !status.Enabled || status.Model != "fake-text" || status.DeciderModel != "fake-decider" {
		t.Fatalf("with a Supervisor: %+v", status)
	}
	if status.Watching.Sessions != 1 || status.Watching.OpenSessions != 1 || len(status.Watching.Surfaces) != 2 {
		t.Fatalf("watching: %+v", status.Watching)
	}
	if status.Today.Decided != 1 || status.Today.Shadowed != 1 || status.Today.Allowed != 0 || status.LastDecisionAt == nil || status.LastJudgementAt != nil {
		t.Fatalf("today: %+v (last %v)", status.Today, status.LastDecisionAt)
	}

	// A configured model names both until a model answers; a triage names
	// the decision model that answered.
	e.SetSupervisor(&fakeProvAPISupervisor{})
	settings := e.Settings(t.Context())
	settings.SupervisorModel = "claude-haiku"
	if err := e.Store().PutSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	e.Invalidate()
	provAPIOK(t, h.SupervisorStatus, http.MethodGet, "/api/provenance/supervisor/status", nil, nil, &status)
	if status.Model != "claude-haiku" || status.DeciderModel != "claude-haiku" {
		t.Fatalf("with a configured model: %+v", status)
	}
	now := time.Now().UTC()
	if err := e.Store().CreateApproval(t.Context(), &provenance.Approval{
		ID: "apr-1", SessionID: s.ID, Status: provenance.ApprovalPending, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
		Triage: &provenance.Triage{Recommendation: "deny", Model: "openai/gpt-5-mini"},
	}); err != nil {
		t.Fatal(err)
	}
	provAPIOK(t, h.SupervisorStatus, http.MethodGet, "/api/provenance/supervisor/status", nil, nil, &status)
	if status.PendingApprovals != 1 || status.Today.Triaged != 1 || status.Model != "claude-haiku" || status.DeciderModel != "openai/gpt-5-mini" {
		t.Fatalf("after an approval: %+v", status)
	}
}

func TestSupervisorJudgedCounts(t *testing.T) {
	cases := []struct {
		name     string
		decision provenance.Decision
		judged   bool
	}{
		{
			name:     "supervisor rationale",
			decision: provenance.Decision{Effect: provenance.EffectAllow, Reason: "Supervisor (gpt, confidence 90%): I allowed it."},
			judged:   true,
		},
		{
			name:     "enforced judge policy",
			decision: provenance.Decision{Effect: provenance.EffectRequireApproval, Matched: []provenance.PolicyMatch{{Effect: provenance.EffectJudge}}},
			judged:   true,
		},
		{
			name:     "shadow judge policy",
			decision: provenance.Decision{Effect: provenance.EffectAllow, Matched: []provenance.PolicyMatch{{Effect: provenance.EffectJudge, Shadow: true}}},
			judged:   false,
		},
		{
			name:     "plain deny",
			decision: provenance.Decision{Effect: provenance.EffectDeny, Reason: "policy fired"},
			judged:   false,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := judgedBySupervisor(&tt.decision); got != tt.judged {
				t.Fatalf("judged = %v, want %v", got, tt.judged)
			}
		})
	}

	snap := &supervisorSnapshot{now: time.Now(), events: []provenance.Event{
		{At: time.Now(), Decision: &provenance.Decision{Allow: false, Effect: provenance.EffectRequireApproval, Reason: "Supervisor (m): I held it."}},
		{At: time.Now(), Decision: &provenance.Decision{Allow: false, Effect: provenance.EffectDeny}},
		{At: time.Now(), Decision: &provenance.Decision{Allow: true, Effect: provenance.EffectAllow}},
	}}
	status := supervisorStatusOf(t.Context(), snap)
	if status.Today.Held != 1 || status.Today.Denied != 1 || status.Today.Allowed != 1 || status.Today.Judged != 1 || status.LastJudgementAt == nil {
		t.Fatalf("counts: %+v", status.Today)
	}
}

func TestSupervisorInsights(t *testing.T) {
	h, e := newProvAPITest(t)

	// Nothing seen yet: one coverage finding and a deterministic briefing.
	var out SupervisorInsights
	provAPIOK(t, h.SupervisorInsights, http.MethodGet, "/api/provenance/supervisor/insights", nil, nil, &out)
	if len(out.Items) != 1 || out.Items[0].Kind != insightCoverageGap || !strings.HasPrefix(out.Briefing, "I haven't seen") || out.BriefingModel != "" {
		t.Fatalf("empty insights: %+v", out)
	}

	s := provAPISeedTrifecta(t, e)
	// A tool call on a heuristically labelled tool.
	if _, _, err := e.Decide(t.Context(), provenance.DecideRequest{Subject: provAPISubject, Session: provAPIKey,
		Action: provenance.Action{Surface: provenance.SurfaceMCP, Kind: provenance.KindToolCall, Server: "github", Tool: "search_issues"}}); err != nil {
		t.Fatal(err)
	}
	// A planted honeytoken leaves the session.
	token := provenance.NewHoneytoken("api-key", "repo .env", "sk-obot-canary-0123456789", "admin", time.Now().UTC())
	if err := e.Store().PutHoneytoken(t.Context(), &token); err != nil {
		t.Fatal(err)
	}
	e.Invalidate()
	if _, d, err := e.Decide(t.Context(), provenance.DecideRequest{Subject: provAPISubject, Session: provAPIKey,
		Action: provenance.Action{Surface: provenance.SurfaceMCP, Kind: provenance.KindToolCall, Server: "http", Tool: "post",
			Arguments: map[string]any{"body": "key=sk-obot-canary-0123456789"}, Sink: provenance.Sink{External: true, Writes: true}}}); err != nil || !d.Honeytoken {
		t.Fatalf("honeytoken decision: %+v (%v)", d, err)
	}
	// An approval that has waited 15 minutes, with a triage.
	now := time.Now().UTC()
	if err := e.Store().CreateApproval(t.Context(), &provenance.Approval{
		ID: "apr-old", SessionID: s.ID, PolicyID: "pol-x", PolicyName: "Hold pushes", Status: provenance.ApprovalPending,
		Action:    provenance.Action{Kind: provenance.KindToolCall, Server: "github", Tool: "push"},
		CreatedAt: now.Add(-15 * time.Minute), ExpiresAt: now.Add(15 * time.Minute),
		Triage: &provenance.Triage{Recommendation: "approve", Rationale: "I recommend approving: it is the user's own fork.", Confidence: 0.9},
	}); err != nil {
		t.Fatal(err)
	}

	fake := &fakeSupervisorAssistant{}
	e.SetSupervisor(fake)
	rec := provAPIOK(t, h.SupervisorInsights, http.MethodGet, "/api/provenance/supervisor/insights?refresh=true", nil, nil, &out)
	if out.Briefing != "I watched one session and held nothing." || out.BriefingModel != "fake-text" || fake.briefs != 1 {
		t.Fatalf("briefing: %q %q (%d calls)", out.Briefing, out.BriefingModel, fake.briefs)
	}
	if !strings.Contains(fake.lastFacts, "Findings") || !strings.Contains(fake.lastFacts, "Honeytoken") {
		t.Fatalf("facts should list the findings: %s", fake.lastFacts)
	}
	for _, content := range []string{"pricing is confidential", "Ada Lovelace", "sk-obot-canary"} {
		if strings.Contains(rec.Body.String(), content) || strings.Contains(fake.lastFacts, content) {
			t.Fatalf("insights must be content-free, found %q", content)
		}
	}
	if out.Items[0].Severity != insightSeverityCritical {
		t.Fatalf("critical findings come first: %+v", out.Items[0])
	}

	honey := provAPIInsight(t, out.Items, insightHoneytokenTripped)
	if honey.Severity != insightSeverityCritical || honey.SessionID != s.ID || honey.Action == nil || honey.Action.Path != "/api/provenance/kills" {
		t.Fatalf("honeytoken insight: %+v", honey)
	}

	shadow := provAPIInsight(t, out.Items, insightShadowPolicy)
	if shadow.PolicyID == "" || shadow.Action == nil || shadow.Action.Method != http.MethodPut || shadow.Action.Path != "/api/provenance/policies/"+shadow.PolicyID {
		t.Fatalf("shadow insight: %+v", shadow)
	}
	// The action's body is the whole policy in enforce mode, accepted as is.
	var body provenance.Policy
	if err := json.Unmarshal(provAPIMarshal(t, shadow.Action.Body), &body); err != nil {
		t.Fatal(err)
	}
	var updated provenance.Policy
	provAPIOK(t, h.UpdatePolicy, http.MethodPut, shadow.Action.Path, body, map[string]string{"id": shadow.PolicyID}, &updated)
	if updated.Mode != provenance.ModeEnforce || updated.Condition == "" {
		t.Fatalf("enforcing from the insight: %+v", updated)
	}

	risky := provAPIInsight(t, out.Items, insightOpenRiskySession)
	if risky.SessionID != s.ID || risky.Action == nil || risky.Action.Method != http.MethodPost || !strings.Contains(risky.Title, "untrusted content and confidential data") {
		t.Fatalf("risky session insight: %+v", risky)
	}
	var kill provenance.Kill
	provAPIOK(t, h.CreateKill, http.MethodPost, risky.Action.Path, risky.Action.Body, nil, &kill)
	if kill.Scope != provenance.KillScopeSession || kill.Target != s.ID {
		t.Fatalf("stopping from the insight: %+v", kill)
	}

	aging := provAPIInsight(t, out.Items, insightApprovalsAging)
	if aging.SessionID != s.ID || aging.Action == nil || aging.Action.Path != "/api/provenance/approvals/apr-old/approve" || !strings.Contains(aging.Detail, "user's own fork") {
		t.Fatalf("aging approval insight: %+v", aging)
	}

	coverage := provAPIInsight(t, out.Items, insightCoverageGap)
	if !strings.Contains(coverage.Title, "llm") || coverage.Action != nil {
		t.Fatalf("coverage insight: %+v", coverage)
	}

	tools := provAPIInsight(t, out.Items, insightUnlabeledTools)
	if !strings.Contains(tools.Detail, "github.search_issues") {
		t.Fatalf("unlabeled tools insight: %+v", tools)
	}

	// Cached for 30 seconds unless refreshed.
	var cached SupervisorInsights
	provAPIOK(t, h.SupervisorInsights, http.MethodGet, "/api/provenance/supervisor/insights", nil, nil, &cached)
	if fake.briefs != 1 || !cached.GeneratedAt.Equal(out.GeneratedAt) {
		t.Fatalf("insights should be cached: %d briefs", fake.briefs)
	}
	// A failed briefing falls back to the deterministic one; the stopped
	// session is no longer a risky open session.
	fake.err = errors.New("model down")
	var after SupervisorInsights
	provAPIOK(t, h.SupervisorInsights, http.MethodGet, "/api/provenance/supervisor/insights?refresh=true", nil, nil, &after)
	if fake.briefs != 2 || after.BriefingModel != "" || !strings.HasPrefix(after.Briefing, "In the last 24 hours I watched 1 session") {
		t.Fatalf("fallback briefing: %q", after.Briefing)
	}
	for _, item := range after.Items {
		if item.Kind == insightOpenRiskySession || item.PolicyID == shadow.PolicyID && item.Kind == insightShadowPolicy {
			t.Fatalf("resolved findings should be gone: %+v", item)
		}
	}
}

func TestSupervisorAsk(t *testing.T) {
	h, e := newProvAPITest(t)
	s := provAPISeedTrifecta(t, e)
	ask := supervisorAskRequest{Question: "What did you stop today?"}

	if code := provAPIStatus(t, h.SupervisorAsk, http.MethodPost, "/api/provenance/supervisor/ask", ask, nil); code != http.StatusServiceUnavailable {
		t.Fatalf("without a Supervisor ask should be a 503, got %d", code)
	}
	rec, err := provAPICall(t, h.SupervisorAsk, http.MethodPost, "/api/provenance/supervisor/ask", ask, nil)
	if err != nil || !strings.Contains(rec.Body.String(), "the Supervisor is not configured") {
		t.Fatalf("503 body: %s (%v)", rec.Body.String(), err)
	}
	// A Supervisor without the optional interface cannot answer either.
	e.SetSupervisor(&fakeProvAPISupervisor{})
	if code := provAPIStatus(t, h.SupervisorAsk, http.MethodPost, "/api/provenance/supervisor/ask", ask, nil); code != http.StatusServiceUnavailable {
		t.Fatalf("a Supervisor that cannot answer should be a 503, got %d", code)
	}

	fake := &fakeSupervisorAssistant{}
	e.SetSupervisor(fake)
	if code := provAPIStatus(t, h.SupervisorAsk, http.MethodPost, "/api/provenance/supervisor/ask", supervisorAskRequest{Question: "  "}, nil); code != http.StatusBadRequest {
		t.Fatalf("an empty question should be a 400, got %d", code)
	}
	if code := provAPIStatus(t, h.SupervisorAsk, http.MethodPost, "/api/provenance/supervisor/ask", supervisorAskRequest{Question: "why?", SessionID: "ses-missing"}, nil); code != http.StatusNotFound {
		t.Fatalf("an unknown session should be a 404, got %d", code)
	}

	var out supervisorAskResponse
	provAPIOK(t, h.SupervisorAsk, http.MethodPost, "/api/provenance/supervisor/ask", ask, nil, &out)
	if out.Answer != "I stopped nothing today." || out.Model != "fake-text" || out.Context.Events != 1 || out.Context.Sessions != 1 {
		t.Fatalf("answer: %+v", out)
	}
	if fake.lastAsk != ask.Question || !strings.Contains(fake.lastContext, "Recent decisions") || strings.Contains(fake.lastContext, "The session the administrator") {
		t.Fatalf("context without a session: %s", fake.lastContext)
	}

	provAPIOK(t, h.SupervisorAsk, http.MethodPost, "/api/provenance/supervisor/ask", supervisorAskRequest{Question: "Why is this risky?", SessionID: s.ID}, nil, &out)
	if out.Context.Sessions != 1 || out.Context.Events < 3 {
		t.Fatalf("session context counts: %+v", out.Context)
	}
	for _, want := range []string{"The session the administrator is asking about: " + s.ID, "Where each label came from", "salesforce.query", "Warrant: none issued", "Its events"} {
		if !strings.Contains(fake.lastContext, want) {
			t.Fatalf("session context should have %q: %s", want, fake.lastContext)
		}
	}
	for _, content := range []string{"pricing is confidential", "Ada Lovelace", "ignore previous instructions"} {
		if strings.Contains(fake.lastContext, content) {
			t.Fatalf("the context must be content-free, found %q", content)
		}
	}

	fake.err = errors.New("model down")
	if code := provAPIStatus(t, h.SupervisorAsk, http.MethodPost, "/api/provenance/supervisor/ask", ask, nil); code != http.StatusBadGateway {
		t.Fatalf("a failed answer should be a 502, got %d", code)
	}

	settings := e.Settings(t.Context())
	settings.SupervisorEnabled = false
	if err := e.Store().PutSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	e.Invalidate()
	if code := provAPIStatus(t, h.SupervisorAsk, http.MethodPost, "/api/provenance/supervisor/ask", ask, nil); code != http.StatusServiceUnavailable {
		t.Fatalf("a disabled Supervisor should be a 503, got %d", code)
	}
}

func TestSupervisorInsightsSkipRedTeam(t *testing.T) {
	now := time.Now().UTC()
	risky := provenance.SessionState{Risk: 90, Labels: provenance.Labels{provenance.LabelUntrusted, provenance.LabelPII}}
	snap := &supervisorSnapshot{
		now:   now,
		since: now.Add(-supervisorWindow),
		sessions: []provenance.Session{
			{ID: "ses-rt", UserID: "redteam-user", StartedAt: now, LastActivityAt: now, State: risky},
			{ID: "ses-real", UserID: "u1", Email: "craig@example.com", Agent: "claude_code", StartedAt: now, LastActivityAt: now, State: risky},
		},
		pending: []provenance.Approval{
			{ID: "apr-rt", SessionID: "ses-rt", UserID: "redteam-user", CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour)},
		},
	}
	for _, item := range supervisorInsightItems(snap) {
		if item.SessionID == "ses-rt" {
			t.Fatalf("red-team sessions are not findings: %+v", item)
		}
	}
	item := provAPIInsight(t, supervisorInsightItems(snap), insightOpenRiskySession)
	if item.SessionID != "ses-real" || item.Severity != insightSeverityCritical || !strings.HasPrefix(item.Title, "craig@example.com's claude_code session") {
		t.Fatalf("real risky session: %+v", item)
	}
	if got := humanDuration(125 * time.Minute); got != "2 h 5 min" {
		t.Fatalf("humanDuration: %q", got)
	}
}
