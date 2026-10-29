package handlers

import (
	"cmp"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/provenance"
)

const (
	provenanceDefaultLimit = 100
	provenanceMaxLimit     = 1000
	// provenanceNarrativeEvents bounds how much of a session the narrative sees.
	provenanceNarrativeEvents = 500
	// provenanceDraftExamples is how many recent decisions the Supervisor sees
	// when drafting a policy.
	provenanceDraftExamples = 24
	// provenanceOverviewSessions bounds the sessions the overview scans for
	// coverage; the total still comes from the store.
	provenanceOverviewSessions = 2000
	// provenanceOverviewEvents bounds the decisions the overview counts.
	provenanceOverviewEvents = 20000
	provenanceOverviewBatch  = 500
	provenanceDefaultHours   = 24
	provenanceMaxHours       = 24 * 366
	provenanceDefaultSince   = 720
	honeytokenEmailDomain    = "honeytoken.obot.ai"
	honeytokenUpperDigits    = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	honeytokenLowerDigits    = "abcdefghijklmnopqrstuvwxyz0123456789"
)

// ProvenanceHandler serves the cross-surface session graph and the Obot
// Policy engine: sessions and their replay, the decisions feed, policies,
// approvals, the kill switch, honeytokens, tool labels and settings.
type ProvenanceHandler struct {
	engine *provenance.Engine
	// insights caches the Supervisor's insights (provenance_supervisor.go).
	insights supervisorInsightsCache
}

// provenanceList is the shape of every list response.
type provenanceList struct {
	Items  any   `json:"items"`
	Total  int64 `json:"total"`
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
}

// ProvenanceSettingsAPI is provenance.Settings with its durations as seconds.
type ProvenanceSettingsAPI struct {
	ApprovedDestinations      []string `json:"approvedDestinations"`
	TrustedHosts              []string `json:"trustedHosts"`
	SupervisorEnabled         bool     `json:"supervisorEnabled"`
	SupervisorModel           string   `json:"supervisorModel,omitempty"`
	SupervisorBudgetSeconds   int      `json:"supervisorBudgetSeconds"`
	DefaultApprovalTTLSeconds int      `json:"defaultApprovalTTLSeconds"`
	RetentionDays             int      `json:"retentionDays"`
	// ContentTagging configures the optional, additive AI content tagger. Its
	// budget is already in seconds, so it maps through unchanged.
	ContentTagging provenance.ContentTagging `json:"contentTagging"`
}

// ProvenanceOverview is the supervision dashboard's summary.
type ProvenanceOverview struct {
	Hours     int                 `json:"hours"`
	Since     time.Time           `json:"since"`
	Coverage  provenance.Coverage `json:"coverage"`
	Decisions struct {
		Allowed          int `json:"allowed"`
		Denied           int `json:"denied"`
		Shadowed         int `json:"shadowed"`
		ApprovalsPending int `json:"approvals_pending"`
	} `json:"decisions"`
	ByEffect  map[string]int       `json:"byEffect"`
	BySurface map[string]int       `json:"bySurface"`
	Riskiest  []provenance.Session `json:"riskiest"`
	Policies  struct {
		Enforce int `json:"enforce"`
		Shadow  int `json:"shadow"`
	} `json:"policies"`
	Supervisor struct {
		Enabled    bool   `json:"enabled"`
		Configured bool   `json:"configured"`
		Model      string `json:"model,omitempty"`
	} `json:"supervisor"`
}

type warrantRequest struct {
	Task       string   `json:"task"`
	Scopes     []string `json:"scopes"`
	TTLSeconds int      `json:"ttlSeconds"`
	Delegation []string `json:"delegation"`
}

type policyValidateRequest struct {
	Condition string `json:"condition"`
}

type policyValidateResponse struct {
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
}

type policySimulateRequest struct {
	Policy     provenance.Policy    `json:"policy"`
	SinceHours int                  `json:"sinceHours"`
	Surfaces   []provenance.Surface `json:"surfaces"`
}

type policyDraftRequest struct {
	Text string `json:"text"`
}

type approvalResolveRequest struct {
	Note string `json:"note"`
}

type killRequest struct {
	Scope      string `json:"scope"`
	Target     string `json:"target"`
	Reason     string `json:"reason"`
	TTLSeconds int    `json:"ttlSeconds"`
}

type honeytokenRequest struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

type honeytokenResponse struct {
	Honeytoken provenance.Honeytoken `json:"honeytoken"`
	// Value is returned once, at creation; only its hash is stored.
	Value string `json:"value"`
}

type toolLabelRequest struct {
	Server string   `json:"server"`
	Tool   string   `json:"tool"`
	Labels []string `json:"labels"`
}

func NewProvenanceHandler(engine *provenance.Engine) *ProvenanceHandler {
	return &ProvenanceHandler{engine: engine}
}

func settingsToAPI(s provenance.Settings) ProvenanceSettingsAPI {
	approved, trusted := s.ApprovedDestinations, s.TrustedHosts
	if approved == nil {
		approved = []string{}
	}
	if trusted == nil {
		trusted = []string{}
	}
	return ProvenanceSettingsAPI{
		ApprovedDestinations:      approved,
		TrustedHosts:              trusted,
		SupervisorEnabled:         s.SupervisorEnabled,
		SupervisorModel:           s.SupervisorModel,
		SupervisorBudgetSeconds:   int(s.SupervisorBudget / time.Second),
		DefaultApprovalTTLSeconds: int(s.DefaultApprovalTTL / time.Second),
		RetentionDays:             s.RetentionDays,
		ContentTagging:            s.ContentTagging,
	}
}

func settingsFromAPI(in ProvenanceSettingsAPI) provenance.Settings {
	return provenance.Settings{
		ApprovedDestinations: in.ApprovedDestinations,
		TrustedHosts:         in.TrustedHosts,
		SupervisorEnabled:    in.SupervisorEnabled,
		SupervisorModel:      in.SupervisorModel,
		SupervisorBudget:     time.Duration(in.SupervisorBudgetSeconds) * time.Second,
		DefaultApprovalTTL:   time.Duration(in.DefaultApprovalTTLSeconds) * time.Second,
		RetentionDays:        in.RetentionDays,
		ContentTagging:       in.ContentTagging,
	}
}

// ---- helpers ----

// stripFingerprints returns a copy of the session without its fingerprints,
// which are internal lineage state and can be large.
func stripFingerprints(s provenance.Session) provenance.Session {
	s.State.Fingerprints = nil
	return s
}

func stripSessions(sessions []provenance.Session) []provenance.Session {
	out := make([]provenance.Session, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, stripFingerprints(s))
	}
	return out
}

// callerIdentity is who resolved an approval or planted a token: the caller's
// email when the authenticator reported one, otherwise their UID.
func callerIdentity(req api.Context) string {
	if req.User == nil {
		return ""
	}
	if email := cmp.Or(req.User.GetExtra()["email"]...); email != "" {
		return email
	}
	return req.User.GetUID()
}

func parseProvenancePaging(q url.Values) (limit, offset int, err error) {
	limit = provenanceDefaultLimit
	if v := q.Get("limit"); v != "" {
		limit, err = strconv.Atoi(v)
		if err != nil || limit < 0 {
			return 0, 0, types.NewErrBadRequest("invalid limit %q", v)
		}
		if limit == 0 || limit > provenanceMaxLimit {
			limit = provenanceMaxLimit
		}
	}
	if v := q.Get("offset"); v != "" {
		offset, err = strconv.Atoi(v)
		if err != nil || offset < 0 {
			return 0, 0, types.NewErrBadRequest("invalid offset %q", v)
		}
	}
	return limit, offset, nil
}

func parseProvenanceTime(q url.Values, key string) (time.Time, error) {
	v := q.Get(key)
	if v == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, types.NewErrBadRequest("invalid %s %q: expected RFC3339", key, v)
	}
	return t, nil
}

func parseProvenanceBool(q url.Values, key string) (*bool, error) {
	v := q.Get(key)
	if v == "" {
		return nil, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return nil, types.NewErrBadRequest("invalid %s %q: expected true or false", key, v)
	}
	return &b, nil
}

func parseProvenanceSurface(v string) (provenance.Surface, error) {
	if v == "" {
		return "", nil
	}
	s := provenance.Surface(v)
	if !provenance.ValidSurface(s) {
		return "", types.NewErrBadRequest("unknown surface %q", v)
	}
	return s, nil
}

// provenanceLookupErr maps a store miss to a 404.
func provenanceLookupErr(err error, what, id string) error {
	if errors.Is(err, provenance.ErrNotFound) {
		return types.NewErrNotFound("%s %q not found", what, id)
	}
	return err
}

func (h *ProvenanceHandler) getSession(req api.Context) (*provenance.Session, error) {
	id := req.PathValue("id")
	s, err := h.engine.Store().GetSession(req.Context(), id)
	if err != nil {
		return nil, provenanceLookupErr(err, "session", id)
	}
	return s, nil
}

func writeNoContent(req api.Context) error {
	req.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- sessions ----

func (h *ProvenanceHandler) ListSessions(req api.Context) error {
	q := req.URL.Query()
	limit, offset, err := parseProvenancePaging(q)
	if err != nil {
		return err
	}
	surface, err := parseProvenanceSurface(q.Get("surface"))
	if err != nil {
		return err
	}
	open, err := parseProvenanceBool(q, "open")
	if err != nil {
		return err
	}
	killed, err := parseProvenanceBool(q, "killed")
	if err != nil {
		return err
	}
	start, err := parseProvenanceTime(q, "start_time")
	if err != nil {
		return err
	}
	end, err := parseProvenanceTime(q, "end_time")
	if err != nil {
		return err
	}
	minRisk := 0
	if v := q.Get("min_risk"); v != "" {
		if minRisk, err = strconv.Atoi(v); err != nil || minRisk < 0 || minRisk > 100 {
			return types.NewErrBadRequest("invalid min_risk %q: expected 0 to 100", v)
		}
	}
	sortBy := q.Get("sort_by")
	switch sortBy {
	case "", "last_activity_at", "started_at", "risk":
	default:
		return types.NewErrBadRequest("invalid sort_by %q: expected last_activity_at, started_at or risk", sortBy)
	}
	sortDesc := true
	switch order := q.Get("sort_order"); order {
	case "", "desc":
	case "asc":
		sortDesc = false
	default:
		return types.NewErrBadRequest("invalid sort_order %q: expected asc or desc", order)
	}

	sessions, total, err := h.engine.Store().ListSessions(req.Context(), provenance.SessionQuery{
		UserID:   q.Get("user_id"),
		Agent:    q.Get("agent"),
		Surface:  surface,
		MinRisk:  minRisk,
		Label:    provenance.Label(q.Get("label")),
		Killed:   killed,
		Open:     open,
		Query:    q.Get("query"),
		Start:    start,
		End:      end,
		Limit:    limit,
		Offset:   offset,
		SortBy:   sortBy,
		SortDesc: sortDesc,
	})
	if err != nil {
		return err
	}
	return req.Write(provenanceList{Items: stripSessions(sessions), Total: total, Limit: limit, Offset: offset})
}

func (h *ProvenanceHandler) GetSession(req api.Context) error {
	s, err := h.getSession(req)
	if err != nil {
		return err
	}
	return req.Write(stripFingerprints(*s))
}

func (h *ProvenanceHandler) ListSessionEvents(req api.Context) error {
	s, err := h.getSession(req)
	if err != nil {
		return err
	}
	q := req.URL.Query()
	limit, offset, err := parseProvenancePaging(q)
	if err != nil {
		return err
	}
	ascending, err := parseProvenanceBool(q, "ascending")
	if err != nil {
		return err
	}
	events, total, err := h.engine.Store().ListEvents(req.Context(), provenance.EventQuery{
		SessionID: s.ID,
		Limit:     limit,
		Offset:    offset,
		Ascending: ascending != nil && *ascending,
	})
	if err != nil {
		return err
	}
	if events == nil {
		events = []provenance.Event{}
	}
	return req.Write(provenanceList{Items: events, Total: total, Limit: limit, Offset: offset})
}

func (h *ProvenanceHandler) SessionNarrative(req api.Context) error {
	s, err := h.getSession(req)
	if err != nil {
		return err
	}
	events, _, err := h.engine.Store().ListEvents(req.Context(), provenance.EventQuery{SessionID: s.ID, Ascending: true, Limit: provenanceNarrativeEvents})
	if err != nil {
		return err
	}
	if sup := h.engine.Supervisor(); sup != nil && h.engine.Settings(req.Context()).SupervisorEnabled {
		if n, err := sup.Narrate(req.Context(), s, events); err == nil {
			return req.Write(n)
		}
	}
	return req.Write(fallbackNarrative(s, events))
}

// fallbackNarrative is the deterministic account of a session used when no
// Supervisor is configured: the session's taint, its counters, and every
// decided event in order.
func fallbackNarrative(s *provenance.Session, events []provenance.Event) provenance.Narrative {
	st := s.State
	explain := st.Explain()
	if explain != "" {
		explain = strings.ToUpper(explain[:1]) + explain[1:]
	}
	n := provenance.Narrative{
		Summary: fmt.Sprintf("%s. %d events, %d tool calls, %d denied, %d shadowed, %d approvals, %d external writes; risk %d.",
			explain, st.Events, st.ToolCalls, st.Denied, st.Shadowed, st.Approvals, st.ExternalOut, st.Risk),
		Timeline: []string{},
		Concerns: []string{},
	}
	if st.Killed {
		n.Concerns = append(n.Concerns, "the session was stopped with the kill switch")
	}
	if st.Warrant != nil && st.Warrant.Drifted {
		n.Concerns = append(n.Concerns, "the session drifted outside its warrant: "+st.Warrant.DriftSummary)
	}
	for _, ev := range events {
		if ev.Decision == nil {
			continue
		}
		line := ev.Summary
		if line == "" && ev.Action != nil {
			line = provenance.ActionSummary(*ev.Action)
		}
		if line == "" {
			line = string(ev.Kind)
		}
		entry := fmt.Sprintf("%s [%s] %s: %s", ev.At.Format(time.RFC3339), ev.Surface, line, ev.Decision.Effect)
		if ev.Decision.Shadow {
			entry += " (shadow)"
		}
		n.Timeline = append(n.Timeline, entry)
		if !ev.Decision.Allow || ev.Decision.Shadow {
			n.Concerns = append(n.Concerns, line+": "+ev.Decision.Reason)
		}
	}
	return n
}

func (h *ProvenanceHandler) IssueWarrant(req api.Context) error {
	id := req.PathValue("id")
	var in warrantRequest
	if err := req.Read(&in); err != nil {
		return types.NewErrBadRequest("invalid request: %v", err)
	}
	if strings.TrimSpace(in.Task) == "" {
		return types.NewErrBadRequest("task is required")
	}
	if in.TTLSeconds < 0 {
		return types.NewErrBadRequest("ttlSeconds must not be negative")
	}
	w, err := h.engine.IssueWarrant(req.Context(), id, in.Task, in.Scopes, time.Duration(in.TTLSeconds)*time.Second, in.Delegation)
	if err != nil {
		return provenanceLookupErr(err, "session", id)
	}
	return req.WriteCreated(w)
}

func (h *ProvenanceHandler) EndSession(req api.Context) error {
	id := req.PathValue("id")
	if err := h.engine.EndSession(req.Context(), id, "session ended by "+callerIdentity(req)); err != nil {
		return provenanceLookupErr(err, "session", id)
	}
	return writeNoContent(req)
}

// ---- decisions feed ----

func (h *ProvenanceHandler) ListEvents(req api.Context) error {
	q := req.URL.Query()
	limit, offset, err := parseProvenancePaging(q)
	if err != nil {
		return err
	}
	surface, err := parseProvenanceSurface(q.Get("surface"))
	if err != nil {
		return err
	}
	denied, err := parseProvenanceBool(q, "denied")
	if err != nil {
		return err
	}
	start, err := parseProvenanceTime(q, "start_time")
	if err != nil {
		return err
	}
	end, err := parseProvenanceTime(q, "end_time")
	if err != nil {
		return err
	}
	decided := true
	events, total, err := h.engine.Store().ListEvents(req.Context(), provenance.EventQuery{
		UserID:  q.Get("user_id"),
		Surface: surface,
		Kind:    provenance.EventKind(q.Get("kind")),
		Decided: &decided,
		Denied:  denied,
		Start:   start,
		End:     end,
		Limit:   limit,
		Offset:  offset,
	})
	if err != nil {
		return err
	}
	if events == nil {
		events = []provenance.Event{}
	}
	return req.Write(provenanceList{Items: events, Total: total, Limit: limit, Offset: offset})
}

// ---- policies ----

func (h *ProvenanceHandler) ListPolicies(req api.Context) error {
	policies, err := h.engine.Store().ListPolicies(req.Context())
	if err != nil {
		return err
	}
	if policies == nil {
		policies = []provenance.Policy{}
	}
	return req.Write(map[string]any{"items": policies})
}

func (h *ProvenanceHandler) getPolicy(req api.Context) (*provenance.Policy, error) {
	id := req.PathValue("id")
	p, err := h.engine.Store().GetPolicy(req.Context(), id)
	if err != nil {
		return nil, provenanceLookupErr(err, "policy", id)
	}
	return p, nil
}

func (h *ProvenanceHandler) GetPolicy(req api.Context) error {
	p, err := h.getPolicy(req)
	if err != nil {
		return err
	}
	return req.Write(p)
}

func (h *ProvenanceHandler) CreatePolicy(req api.Context) error {
	var p provenance.Policy
	if err := req.Read(&p); err != nil {
		return types.NewErrBadRequest("invalid request: %v", err)
	}
	now := time.Now().UTC()
	p.ID = provenance.NewID("pol")
	p.BuiltIn = false
	p.CreatedAt, p.UpdatedAt = now, now
	if err := p.Validate(); err != nil {
		return types.NewErrBadRequest("invalid policy: %v", err)
	}
	if err := h.engine.Store().PutPolicy(req.Context(), &p); err != nil {
		return err
	}
	h.engine.Invalidate()
	return req.WriteCreated(p)
}

func (h *ProvenanceHandler) UpdatePolicy(req api.Context) error {
	existing, err := h.getPolicy(req)
	if err != nil {
		return err
	}
	var p provenance.Policy
	if err := req.Read(&p); err != nil {
		return types.NewErrBadRequest("invalid request: %v", err)
	}
	// Identity and provenance of the record are not the caller's to change.
	p.ID = existing.ID
	p.BuiltIn = existing.BuiltIn
	p.CreatedAt = existing.CreatedAt
	p.UpdatedAt = time.Now().UTC()
	if err := p.Validate(); err != nil {
		return types.NewErrBadRequest("invalid policy: %v", err)
	}
	if err := h.engine.Store().PutPolicy(req.Context(), &p); err != nil {
		return err
	}
	h.engine.Invalidate()
	return req.Write(p)
}

func (h *ProvenanceHandler) DeletePolicy(req api.Context) error {
	p, err := h.getPolicy(req)
	if err != nil {
		return err
	}
	if p.BuiltIn {
		return types.NewErrBadRequest("built-in policy %q cannot be deleted; disable it instead", p.ID)
	}
	if err := h.engine.Store().DeletePolicy(req.Context(), p.ID); err != nil {
		return err
	}
	h.engine.Invalidate()
	return writeNoContent(req)
}

func (h *ProvenanceHandler) ValidatePolicy(req api.Context) error {
	var in policyValidateRequest
	if err := req.Read(&in); err != nil {
		return types.NewErrBadRequest("invalid request: %v", err)
	}
	probe := provenance.Policy{Name: "validate", Condition: in.Condition, Effect: provenance.EffectAllow, Mode: provenance.ModeShadow}
	if err := probe.Validate(); err != nil {
		return req.Write(policyValidateResponse{Valid: false, Error: err.Error()})
	}
	return req.Write(policyValidateResponse{Valid: true})
}

func (h *ProvenanceHandler) SimulatePolicy(req api.Context) error {
	var in policySimulateRequest
	if err := req.Read(&in); err != nil {
		return types.NewErrBadRequest("invalid request: %v", err)
	}
	if in.SinceHours < 0 {
		return types.NewErrBadRequest("sinceHours must not be negative")
	}
	if in.SinceHours == 0 {
		in.SinceHours = provenanceDefaultSince
	}
	for _, s := range in.Surfaces {
		if !provenance.ValidSurface(s) {
			return types.NewErrBadRequest("unknown surface %q", s)
		}
	}
	if err := in.Policy.Validate(); err != nil {
		return types.NewErrBadRequest("invalid policy: %v", err)
	}
	since := time.Now().UTC().Add(-time.Duration(in.SinceHours) * time.Hour)
	res, err := h.engine.Simulate(req.Context(), in.Policy, since, in.Surfaces)
	if err != nil {
		return err
	}
	if res.Samples == nil {
		res.Samples = []provenance.Event{}
	}
	return req.Write(res)
}

func (h *ProvenanceHandler) DraftPolicy(req api.Context) error {
	var in policyDraftRequest
	if err := req.Read(&in); err != nil {
		return types.NewErrBadRequest("invalid request: %v", err)
	}
	if strings.TrimSpace(in.Text) == "" {
		return types.NewErrBadRequest("text is required")
	}
	sup := h.engine.Supervisor()
	if sup == nil {
		return req.WriteCode(map[string]string{"error": "the Supervisor is not configured"}, http.StatusServiceUnavailable)
	}
	if !h.engine.Settings(req.Context()).SupervisorEnabled {
		return req.WriteCode(map[string]string{"error": "the Supervisor is disabled in settings"}, http.StatusServiceUnavailable)
	}
	decided := true
	examples, _, err := h.engine.Store().ListEvents(req.Context(), provenance.EventQuery{Decided: &decided, Limit: provenanceDraftExamples})
	if err != nil {
		return err
	}
	draft, err := sup.DraftPolicy(req.Context(), in.Text, examples)
	if err != nil {
		return types.NewErrHTTP(http.StatusBadGateway, "the Supervisor could not draft a policy: "+err.Error())
	}
	if draft.Policy.Source == "" {
		draft.Policy.Source = in.Text
	}
	if draft.Policy.Mode == "" {
		draft.Policy.Mode = provenance.ModeShadow
	}
	// The draft's own test cases run here so the administrator sees what the
	// condition will and won't catch before saving it.
	for i := range draft.Tests {
		test := &draft.Tests[i]
		if test.Input.Session == nil {
			test.Input.Session = &provenance.Session{}
		}
		if test.Input.Now.IsZero() {
			test.Input.Now = time.Now().UTC()
		}
		actual, err := provenance.EvalPolicy(&draft.Policy, test.Input)
		if err != nil {
			test.Error = err.Error()
			continue
		}
		test.Actual = actual
	}
	if draft.Tests == nil {
		draft.Tests = []provenance.PolicyTest{}
	}
	return req.Write(draft)
}

// ---- approvals ----

func (h *ProvenanceHandler) ListApprovals(req api.Context) error {
	q := req.URL.Query()
	limit, offset, err := parseProvenancePaging(q)
	if err != nil {
		return err
	}
	status := q.Get("status")
	switch status {
	case "", provenance.ApprovalPending, provenance.ApprovalApproved, provenance.ApprovalDenied, provenance.ApprovalExpired:
	default:
		return types.NewErrBadRequest("invalid status %q: expected pending, approved, denied or expired", status)
	}
	approvals, total, err := h.engine.Store().ListApprovals(req.Context(), provenance.ApprovalQuery{
		Status:    status,
		UserID:    q.Get("user_id"),
		SessionID: q.Get("session_id"),
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return err
	}
	if approvals == nil {
		approvals = []provenance.Approval{}
	}
	return req.Write(provenanceList{Items: approvals, Total: total, Limit: limit, Offset: offset})
}

func (h *ProvenanceHandler) GetApproval(req api.Context) error {
	id := req.PathValue("id")
	a, err := h.engine.Store().GetApproval(req.Context(), id)
	if err != nil {
		return provenanceLookupErr(err, "approval", id)
	}
	return req.Write(a)
}

func (h *ProvenanceHandler) ApproveApproval(req api.Context) error {
	return h.resolveApproval(req, true)
}

func (h *ProvenanceHandler) DenyApproval(req api.Context) error {
	return h.resolveApproval(req, false)
}

func (h *ProvenanceHandler) resolveApproval(req api.Context, approve bool) error {
	id := req.PathValue("id")
	var in approvalResolveRequest
	if req.ContentLength != 0 {
		if err := req.Read(&in); err != nil {
			return types.NewErrBadRequest("invalid request: %v", err)
		}
	}
	a, err := h.engine.ResolveApproval(req.Context(), id, approve, callerIdentity(req), in.Note)
	if err != nil {
		if errors.Is(err, provenance.ErrNotFound) {
			return types.NewErrNotFound("approval %q not found", id)
		}
		if a != nil {
			// The approval exists but is no longer pending.
			return types.NewErrHTTP(http.StatusConflict, err.Error())
		}
		return err
	}
	return req.Write(a)
}

// ---- kill switch ----

func (h *ProvenanceHandler) ListKills(req api.Context) error {
	kills, err := h.engine.Store().ListKills(req.Context())
	if err != nil {
		return err
	}
	if kills == nil {
		kills = []provenance.Kill{}
	}
	return req.Write(map[string]any{"items": kills})
}

func (h *ProvenanceHandler) CreateKill(req api.Context) error {
	var in killRequest
	if err := req.Read(&in); err != nil {
		return types.NewErrBadRequest("invalid request: %v", err)
	}
	if in.TTLSeconds < 0 {
		return types.NewErrBadRequest("ttlSeconds must not be negative")
	}
	now := time.Now().UTC()
	k := provenance.Kill{Scope: in.Scope, Target: in.Target, Reason: in.Reason, CreatedBy: callerIdentity(req), CreatedAt: now}
	if in.TTLSeconds > 0 {
		k.ExpiresAt = now.Add(time.Duration(in.TTLSeconds) * time.Second)
	}
	created, err := h.engine.Kill(req.Context(), k)
	if err != nil {
		return types.NewErrBadRequest("%v", err)
	}
	return req.WriteCreated(created)
}

func (h *ProvenanceHandler) DeleteKill(req api.Context) error {
	if err := h.engine.Unkill(req.Context(), req.PathValue("id")); err != nil {
		return err
	}
	return writeNoContent(req)
}

// ---- honeytokens ----

func (h *ProvenanceHandler) ListHoneytokens(req api.Context) error {
	tokens, err := h.engine.Store().ListHoneytokens(req.Context())
	if err != nil {
		return err
	}
	if tokens == nil {
		tokens = []provenance.Honeytoken{}
	}
	return req.Write(map[string]any{"items": tokens})
}

// generateHoneytokenValue makes a believable canary for the kind, or returns
// "" for kinds whose value the administrator must supply.
func generateHoneytokenValue(kind string) (string, error) {
	switch kind {
	case "aws-key":
		suffix, err := randomString(honeytokenUpperDigits, 16)
		return "AKIA" + suffix, err
	case "api-key":
		suffix, err := randomString(honeytokenLowerDigits, 32)
		return "sk-obot-" + suffix, err
	case "email":
		local, err := randomString(honeytokenLowerDigits, 8)
		return "canary-" + local + "@" + honeytokenEmailDomain, err
	}
	return "", nil
}

func randomString(alphabet string, n int) (string, error) {
	out := make([]byte, n)
	maxIndex := big.NewInt(int64(len(alphabet)))
	for i := range out {
		idx, err := rand.Int(rand.Reader, maxIndex)
		if err != nil {
			return "", err
		}
		out[i] = alphabet[idx.Int64()]
	}
	return string(out), nil
}

func (h *ProvenanceHandler) CreateHoneytoken(req api.Context) error {
	var in honeytokenRequest
	if err := req.Read(&in); err != nil {
		return types.NewErrBadRequest("invalid request: %v", err)
	}
	switch in.Kind {
	case "aws-key", "api-key", "email", "record", "custom":
	default:
		return types.NewErrBadRequest("invalid kind %q: expected aws-key, api-key, email, record or custom", in.Kind)
	}
	if strings.TrimSpace(in.Name) == "" {
		return types.NewErrBadRequest("name is required")
	}
	value := strings.TrimSpace(in.Value)
	if value == "" {
		generated, err := generateHoneytokenValue(in.Kind)
		if err != nil {
			return err
		}
		if generated == "" {
			return types.NewErrBadRequest("a value is required for a %s honeytoken", in.Kind)
		}
		value = generated
	}
	if len(value) < 8 {
		return types.NewErrBadRequest("a honeytoken value must be at least 8 characters so it is recognized reliably")
	}
	token := provenance.NewHoneytoken(in.Kind, in.Name, value, callerIdentity(req), time.Now().UTC())
	if err := h.engine.Store().PutHoneytoken(req.Context(), &token); err != nil {
		return err
	}
	h.engine.Invalidate()
	return req.WriteCreated(honeytokenResponse{Honeytoken: token, Value: value})
}

func (h *ProvenanceHandler) DeleteHoneytoken(req api.Context) error {
	if err := h.engine.Store().DeleteHoneytoken(req.Context(), req.PathValue("id")); err != nil {
		return err
	}
	h.engine.Invalidate()
	return writeNoContent(req)
}

// ---- tool labels ----

func (h *ProvenanceHandler) ListToolLabels(req api.Context) error {
	labels, err := h.engine.Store().ListToolLabels(req.Context())
	if err != nil {
		return err
	}
	if labels == nil {
		labels = []provenance.ToolLabel{}
	}
	return req.Write(map[string]any{"items": labels})
}

func (h *ProvenanceHandler) PutToolLabel(req api.Context) error {
	var in toolLabelRequest
	if err := req.Read(&in); err != nil {
		return types.NewErrBadRequest("invalid request: %v", err)
	}
	if strings.TrimSpace(in.Tool) == "" {
		return types.NewErrBadRequest("tool is required")
	}
	var labels provenance.Labels
	for _, l := range in.Labels {
		label := provenance.Label(strings.TrimSpace(l))
		if label == "" {
			continue
		}
		if !slices.Contains(provenance.CapabilityLabels, label) {
			return types.NewErrBadRequest("unknown capability label %q: expected one of %s", l, provenance.CapabilityLabels.String())
		}
		labels = labels.Add(label)
	}
	if labels == nil {
		labels = provenance.Labels{}
	}
	label := provenance.ToolLabel{Server: in.Server, Tool: in.Tool, Labels: labels, Source: "admin", UpdatedAt: time.Now().UTC()}
	if err := h.engine.Store().PutToolLabel(req.Context(), &label); err != nil {
		return err
	}
	h.engine.Invalidate()
	return req.Write(label)
}

// ---- settings ----

func (h *ProvenanceHandler) loadSettings(req api.Context) (provenance.Settings, error) {
	settings, err := h.engine.Store().GetSettings(req.Context())
	if errors.Is(err, provenance.ErrNotFound) {
		return provenance.DefaultSettings(), nil
	}
	return settings, err
}

func (h *ProvenanceHandler) GetSettings(req api.Context) error {
	settings, err := h.loadSettings(req)
	if err != nil {
		return err
	}
	return req.Write(settingsToAPI(settings))
}

func (h *ProvenanceHandler) UpdateSettings(req api.Context) error {
	var in ProvenanceSettingsAPI
	if err := req.Read(&in); err != nil {
		return types.NewErrBadRequest("invalid request: %v", err)
	}
	if in.RetentionDays < 0 {
		return types.NewErrBadRequest("retentionDays must not be negative")
	}
	if in.SupervisorBudgetSeconds < 0 || in.DefaultApprovalTTLSeconds < 0 {
		return types.NewErrBadRequest("durations must not be negative")
	}
	settings := settingsFromAPI(in)
	if err := h.engine.Store().PutSettings(req.Context(), settings); err != nil {
		return err
	}
	h.engine.Invalidate()
	return req.Write(settingsToAPI(settings))
}

// ---- overview ----

func (h *ProvenanceHandler) Overview(req api.Context) error {
	ctx := req.Context()
	hours := provenanceDefaultHours
	if v := req.URL.Query().Get("hours"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 || parsed > provenanceMaxHours {
			return types.NewErrBadRequest("invalid hours %q: expected 1 to %d", v, provenanceMaxHours)
		}
		hours = parsed
	}
	now := time.Now().UTC()
	since := now.Add(-time.Duration(hours) * time.Hour)
	store := h.engine.Store()

	out := ProvenanceOverview{
		Hours:     hours,
		Since:     since,
		ByEffect:  map[string]int{},
		BySurface: map[string]int{},
		Riskiest:  []provenance.Session{},
	}

	sessions, total, err := store.ListSessions(ctx, provenance.SessionQuery{Start: since, Limit: provenanceOverviewSessions, SortDesc: true})
	if err != nil {
		return err
	}
	out.Coverage = provenance.CoverageOf(sessions)
	out.Coverage.Sessions = int(total)

	riskiest, _, err := store.ListSessions(ctx, provenance.SessionQuery{Start: since, SortBy: "risk", SortDesc: true, Limit: 10})
	if err != nil {
		return err
	}
	out.Riskiest = stripSessions(riskiest)

	decided := true
	for offset := 0; offset < provenanceOverviewEvents; {
		events, _, err := store.ListEvents(ctx, provenance.EventQuery{Decided: &decided, Start: since, Limit: provenanceOverviewBatch, Offset: offset})
		if err != nil {
			return err
		}
		if len(events) == 0 {
			break
		}
		offset += len(events)
		for _, ev := range events {
			if ev.Decision == nil {
				continue
			}
			out.ByEffect[string(ev.Decision.Effect)]++
			out.BySurface[string(ev.Surface)]++
			switch {
			case !ev.Decision.Allow:
				out.Decisions.Denied++
			case ev.Decision.Shadow:
				out.Decisions.Shadowed++
			default:
				out.Decisions.Allowed++
			}
		}
	}

	_, pending, err := store.ListApprovals(ctx, provenance.ApprovalQuery{Status: provenance.ApprovalPending, Limit: 1})
	if err != nil {
		return err
	}
	out.Decisions.ApprovalsPending = int(pending)

	policies, err := store.ListPolicies(ctx)
	if err != nil {
		return err
	}
	for _, p := range policies {
		if !p.Enabled {
			continue
		}
		if p.Mode == provenance.ModeEnforce {
			out.Policies.Enforce++
		} else {
			out.Policies.Shadow++
		}
	}

	settings := h.engine.Settings(ctx)
	out.Supervisor.Configured = h.engine.Supervisor() != nil
	out.Supervisor.Enabled = out.Supervisor.Configured && settings.SupervisorEnabled
	out.Supervisor.Model = settings.SupervisorModel
	return req.Write(out)
}
