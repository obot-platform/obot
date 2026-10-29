package provenance

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// cacheTTL bounds how long policy, kill, honeytoken and settings changes
	// take to apply on other replicas.
	cacheTTL = 5 * time.Second
	// sessionCacheIdle is how long an idle session stays in memory.
	sessionCacheIdle = time.Hour
	// recentEventsForJudge is how many events the Supervisor sees.
	recentEventsForJudge = 24
	// approvalPoll is how often AwaitApproval checks the store for a
	// resolution made on another replica.
	approvalPoll = 500 * time.Millisecond
)

// Engine is the single entry point the surfaces call. It resolves sessions,
// ingests content, decides actions, and records everything as events.
type Engine struct {
	store      Store
	supervisor Supervisor
	now        func() time.Time
	logger     *slog.Logger

	lock     sync.Mutex
	sessions map[string]*cachedSession

	refLock     sync.Mutex
	refLoadedAt time.Time
	policies    []Policy
	kills       []Kill
	honeytokens []Honeytoken
	toolLabels  map[string]ToolLabel
	settings    Settings

	waitersLock sync.Mutex
	waiters     map[string][]chan Approval

	classifyLock    sync.Mutex
	classifyPending map[string]bool
}

type cachedSession struct {
	session  *Session
	lastUsed time.Time
}

// Option configures an engine.
type Option func(*Engine)

// IngestRequest is content entering a session's context: a tool result, a
// file read, a web page, the user's own message.
type IngestRequest struct {
	Subject Subject
	Session SessionKey
	Surface Surface
	// Kind is tool.result, file.read or llm.turn.
	Kind     EventKind
	ParentID string
	Tool     string
	Server   string
	ServerID string
	// Capabilities are the labels of the tool that produced the content; they
	// decide whether the content is untrusted or private.
	Capabilities Labels
	// Host is where web or egress content came from, for the trusted-host
	// check.
	Host string
	// Content is scanned for sensitivity labels and fingerprinted. It is not
	// stored.
	Content string
	// Labels are labels the adapter already knows apply.
	Labels  Labels
	Ref     SurfaceRef
	Summary string
	Bytes   int
}

// DecideRequest is an action a surface is about to let an agent perform.
type DecideRequest struct {
	Subject  Subject
	Session  SessionKey
	Action   Action
	ParentID string
	Ref      SurfaceRef
	Summary  string
	// Budget bounds the decision. A Supervisor call that cannot fit is skipped
	// and the judge effect degrades to require_approval.
	Budget time.Duration
	// WaitForApproval makes Decide block until a required approval is
	// resolved or the budget runs out, for surfaces that can hold a request.
	WaitForApproval bool
}

// Coverage summarizes which surfaces have reported into a set of sessions.
type Coverage struct {
	Sessions int             `json:"sessions"`
	Surfaces map[Surface]int `json:"surfaces"`
	// Blind counts sessions observed on only one surface.
	Blind int `json:"blind"`
}

// WithSupervisor sets the Supervisor model.
func WithSupervisor(s Supervisor) Option {
	return func(e *Engine) { e.supervisor = s }
}

// WithClock sets the clock, for tests.
func WithClock(now func() time.Time) Option {
	return func(e *Engine) { e.now = now }
}

// WithLogger sets the logger.
func WithLogger(l *slog.Logger) Option {
	return func(e *Engine) { e.logger = l }
}

// New creates an engine over a store.
func New(store Store, opts ...Option) *Engine {
	e := &Engine{
		store:           store,
		now:             func() time.Time { return time.Now().UTC() },
		logger:          slog.Default(),
		sessions:        map[string]*cachedSession{},
		waiters:         map[string][]chan Approval{},
		classifyPending: map[string]bool{},
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// Store returns the engine's store, for handlers that list and update
// records directly.
func (e *Engine) Store() Store { return e.store }

// Supervisor returns the Supervisor, which may be nil.
func (e *Engine) Supervisor() Supervisor { return e.supervisor }

// SetSupervisor installs a Supervisor after construction.
func (e *Engine) SetSupervisor(s Supervisor) { e.supervisor = s }

// Invalidate makes the next call reload policies, kills, honeytokens, tool
// labels and settings.
func (e *Engine) Invalidate() {
	e.refLock.Lock()
	e.refLoadedAt = time.Time{}
	e.refLock.Unlock()
}

// Forget drops a session from the in-memory cache so the next call reloads
// it from the store.
func (e *Engine) Forget(sessionID string) {
	e.lock.Lock()
	delete(e.sessions, sessionID)
	e.lock.Unlock()
}

// Seed installs the built-in policies and default settings when the store
// has none. It is called once at startup.
func (e *Engine) Seed(ctx context.Context) error {
	policies, err := e.store.ListPolicies(ctx)
	if err != nil {
		return err
	}
	existing := map[string]bool{}
	for _, p := range policies {
		existing[p.ID] = true
	}
	for _, p := range BuiltInPolicies(e.now()) {
		if existing[p.ID] {
			continue
		}
		if err := e.store.PutPolicy(ctx, &p); err != nil {
			return err
		}
	}
	if _, err := e.store.GetSettings(ctx); errors.Is(err, ErrNotFound) {
		if err := e.store.PutSettings(ctx, DefaultSettings()); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	e.Invalidate()
	return nil
}

func (e *Engine) loadRefs(ctx context.Context) error {
	e.refLock.Lock()
	defer e.refLock.Unlock()
	if time.Since(e.refLoadedAt) < cacheTTL {
		return nil
	}
	policies, err := e.store.ListPolicies(ctx)
	if err != nil {
		return err
	}
	kills, err := e.store.ListKills(ctx)
	if err != nil {
		return err
	}
	tokens, err := e.store.ListHoneytokens(ctx)
	if err != nil {
		return err
	}
	labels, err := e.store.ListToolLabels(ctx)
	if err != nil {
		return err
	}
	settings, err := e.store.GetSettings(ctx)
	if errors.Is(err, ErrNotFound) {
		settings = DefaultSettings()
	} else if err != nil {
		return err
	}
	byTool := make(map[string]ToolLabel, len(labels))
	for _, l := range labels {
		byTool[toolLabelKey(l.Server, l.Tool)] = l
	}
	e.policies, e.kills, e.honeytokens, e.toolLabels, e.settings = policies, kills, tokens, byTool, settings
	e.refLoadedAt = time.Now()
	return nil
}

func toolLabelKey(server, tool string) string {
	return strings.ToLower(server) + "\x00" + strings.ToLower(tool)
}

// Settings returns the current settings.
func (e *Engine) Settings(ctx context.Context) Settings {
	if err := e.loadRefs(ctx); err != nil {
		e.logger.Warn("provenance: loading settings", "error", err)
		return DefaultSettings()
	}
	e.refLock.Lock()
	defer e.refLock.Unlock()
	return e.settings
}

// Policies returns the current policies.
func (e *Engine) Policies(ctx context.Context) ([]Policy, error) {
	if err := e.loadRefs(ctx); err != nil {
		return nil, err
	}
	e.refLock.Lock()
	defer e.refLock.Unlock()
	return slices.Clone(e.policies), nil
}

// IsExternal reports whether a destination host is outside the approved
// destinations. With no approved destinations configured every host is
// external, which is the conservative default for shadow-mode policies.
func (e *Engine) IsExternal(ctx context.Context, host string) bool {
	if host == "" {
		return false
	}
	return !HostMatches(host, e.Settings(ctx).ApprovedDestinations)
}

// IsTrustedHost reports whether content from a host is first-party.
func (e *Engine) IsTrustedHost(ctx context.Context, host string) bool {
	return host != "" && HostMatches(host, e.Settings(ctx).TrustedHosts)
}

// ToolCapabilities returns the capability labels of a tool: the administrator's
// or Supervisor's labelling when there is one, otherwise the heuristic
// labelling, which is stored and queued for the Supervisor to refine.
func (e *Engine) ToolCapabilities(ctx context.Context, server, tool, description string) Labels {
	if err := e.loadRefs(ctx); err != nil {
		e.logger.Warn("provenance: loading tool labels", "error", err)
		return ToolCapabilityHeuristics(server, tool, description)
	}
	key := toolLabelKey(server, tool)
	e.refLock.Lock()
	l, ok := e.toolLabels[key]
	e.refLock.Unlock()
	if ok {
		return l.Labels
	}
	caps := ToolCapabilityHeuristics(server, tool, description)
	label := ToolLabel{Server: server, Tool: tool, Labels: caps, Source: "heuristic", UpdatedAt: e.now()}
	e.refLock.Lock()
	e.toolLabels[key] = label
	e.refLock.Unlock()
	if err := e.store.PutToolLabel(ctx, &label); err != nil {
		e.logger.Warn("provenance: storing tool label", "error", err)
	}
	e.classifyAsync(server, tool, description, caps)
	return caps
}

// classifyAsync asks the Supervisor to refine a heuristic labelling, once per
// tool per process.
func (e *Engine) classifyAsync(server, tool, description string, heuristic Labels) {
	if e.supervisor == nil {
		return
	}
	key := toolLabelKey(server, tool)
	e.classifyLock.Lock()
	if e.classifyPending[key] {
		e.classifyLock.Unlock()
		return
	}
	e.classifyPending[key] = true
	e.classifyLock.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if !e.Settings(ctx).SupervisorEnabled {
			return
		}
		labels, rationale, err := e.supervisor.ClassifyTool(ctx, ToolDescriptor{Server: server, Tool: tool, Description: description, Heuristic: heuristic})
		if err != nil {
			e.logger.Debug("provenance: supervisor tool classification failed", "server", server, "tool", tool, "error", err)
			return
		}
		e.refLock.Lock()
		current, ok := e.toolLabels[key]
		e.refLock.Unlock()
		if ok && current.Source == "admin" {
			return
		}
		label := ToolLabel{Server: server, Tool: tool, Labels: labels, Source: "supervisor", Rationale: rationale, UpdatedAt: e.now()}
		if err := e.store.PutToolLabel(ctx, &label); err != nil {
			e.logger.Warn("provenance: storing supervisor tool label", "error", err)
			return
		}
		e.refLock.Lock()
		e.toolLabels[key] = label
		e.refLock.Unlock()
	}()
}

// session returns a session by ID from the cache or the store.
func (e *Engine) session(ctx context.Context, id string) (*Session, error) {
	if id == "" {
		return nil, ErrNotFound
	}
	now := e.now()
	e.lock.Lock()
	if c, ok := e.sessions[id]; ok {
		c.lastUsed = now
		e.lock.Unlock()
		return c.session, nil
	}
	e.lock.Unlock()

	s, err := e.store.GetSession(ctx, id)
	if err != nil {
		return nil, err
	}
	e.cache(s)
	return s, nil
}

func (e *Engine) cache(s *Session) {
	now := e.now()
	e.lock.Lock()
	defer e.lock.Unlock()
	e.sessions[s.ID] = &cachedSession{session: s, lastUsed: now}
	if len(e.sessions) > 1024 {
		for id, c := range e.sessions {
			if now.Sub(c.lastUsed) > sessionCacheIdle {
				delete(e.sessions, id)
			}
		}
	}
}

// Resolve returns the session for a key, creating one when none matches.
func (e *Engine) Resolve(ctx context.Context, subject Subject, key SessionKey, surface Surface) (*Session, error) {
	s, _, err := e.resolve(ctx, subject, key, surface)
	return s, err
}

func (e *Engine) resolve(ctx context.Context, subject Subject, key SessionKey, surface Surface) (*Session, bool, error) {
	now := e.now()
	if key.Agent == "" {
		key.Agent = subject.Actor.Agent
	}
	if key.UserID == "" {
		key.UserID = subject.UserID
	}
	if key.DeviceID == "" {
		key.DeviceID = subject.Actor.DeviceID
	}
	if key.APIKeyID == "" {
		key.APIKeyID = subject.Actor.APIKeyID
	}
	if key.EnvironmentID == "" {
		key.EnvironmentID = subject.Actor.EnvironmentID
	}

	if key.Explicit != "" {
		if s, err := e.session(ctx, key.Explicit); err == nil {
			return e.touch(s, now, surface), false, nil
		} else if !errors.Is(err, ErrNotFound) {
			return nil, false, err
		}
	}
	linkKey := key.LinkKey()
	if linkKey != "" {
		id, err := e.store.ResolveLink(ctx, linkKey)
		if err != nil {
			return nil, false, err
		}
		if id != "" {
			if s, err := e.session(ctx, id); err == nil && s.EndedAt == nil {
				return e.touch(s, now, surface), false, nil
			} else if err != nil && !errors.Is(err, ErrNotFound) {
				return nil, false, err
			}
		}
	}
	fallbackKey := key.FallbackKey()
	if linkKey == "" {
		id, err := e.store.ResolveLink(ctx, fallbackKey)
		if err != nil {
			return nil, false, err
		}
		if id != "" {
			if s, err := e.session(ctx, id); err == nil && !s.Idle(now) {
				return e.touch(s, now, surface), false, nil
			} else if err != nil && !errors.Is(err, ErrNotFound) {
				return nil, false, err
			}
		}
	}

	s := NewSession(subject, key, now)
	s.State.Surfaces = []Surface{surface}
	if err := e.store.PutSession(ctx, s); err != nil {
		return nil, false, err
	}
	if linkKey != "" {
		if err := e.store.PutLink(ctx, linkKey, s.ID); err != nil {
			return nil, false, err
		}
	}
	if err := e.store.PutLink(ctx, fallbackKey, s.ID); err != nil {
		return nil, false, err
	}
	e.cache(s)
	start := Event{
		ID: NewID("evt"), SessionID: s.ID, Surface: surface, Kind: KindSessionStart, At: now,
		UserID: subject.UserID, Actor: subject.Actor, Summary: "session started",
	}
	s.State.Events++
	if err := e.store.AppendEvents(ctx, []Event{start}); err != nil {
		return nil, false, err
	}
	return s, true, nil
}

func (e *Engine) touch(s *Session, now time.Time, surface Surface) *Session {
	e.lock.Lock()
	defer e.lock.Unlock()
	s.Touch(now, surface)
	return s
}

// Ingest records content entering a session and updates its taint.
func (e *Engine) Ingest(ctx context.Context, in IngestRequest) (*Event, error) {
	s, _, err := e.resolve(ctx, in.Subject, in.Session, in.Surface)
	if err != nil {
		return nil, err
	}
	now := e.now()
	if in.Kind == "" {
		in.Kind = KindToolResult
	}
	class := Classify(in.Content)
	labels := in.Labels.Add(class.Labels...)
	if in.Capabilities.Has(CapReadsUntrusted) && (in.Host == "" || !e.IsTrustedHost(ctx, in.Host)) {
		labels = labels.Add(LabelUntrusted)
	}
	if in.Capabilities.Has(CapReadsPrivate) {
		labels = labels.Add(LabelConfidential)
	}
	if in.Capabilities.Has(CapCredentialBearing) {
		labels = labels.Add(LabelSecret)
	}
	if hit := e.honeytokenHit(ctx, in.Content); hit != nil {
		labels = labels.Add(LabelHoneytoken)
	}
	labels = labels.Normalize()

	summary := in.Summary
	if summary == "" {
		summary = ingestSummary(in)
	}
	ev := Event{
		ID: NewID("evt"), SessionID: s.ID, Surface: in.Surface, Kind: in.Kind, At: now,
		UserID: in.Subject.UserID, Actor: in.Subject.Actor, ParentID: in.ParentID,
		Labels: labels, Ref: in.Ref, Summary: summary, Bytes: cmpOr(in.Bytes, len(in.Content)),
		Action: &Action{Surface: in.Surface, Kind: in.Kind, Tool: in.Tool, Server: in.Server, ServerID: in.ServerID, Host: in.Host, Capabilities: in.Capabilities},
	}

	e.lock.Lock()
	s.State.AddLabels(labels, LabelOrigin{EventID: ev.ID, Summary: summary, At: now})
	s.State.AddFingerprints(FingerprintContent(ev.ID, in.Content, labels, class))
	s.State.Events++
	s.State.LastSummary = summary
	s.State.Recompute()
	snapshot := *s
	e.lock.Unlock()

	if err := e.persist(ctx, &snapshot, ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

func cmpOr[T comparable](a, b T) T {
	var zero T
	if a != zero {
		return a
	}
	return b
}

func ingestSummary(in IngestRequest) string {
	switch {
	case in.Tool != "" && in.Server != "":
		return in.Server + "." + in.Tool + " result"
	case in.Tool != "":
		return in.Tool + " result"
	case in.Host != "":
		return "content from " + in.Host
	}
	return string(in.Kind)
}

// Decide decides an action, records it, and returns the decision the surface
// must act on.
func (e *Engine) Decide(ctx context.Context, in DecideRequest) (*Event, Decision, error) {
	start := e.now()
	s, _, err := e.resolve(ctx, in.Subject, in.Session, in.Action.Surface)
	if err != nil {
		return nil, Decision{}, err
	}
	if err := e.loadRefs(ctx); err != nil {
		return nil, Decision{}, err
	}
	a := in.Action
	if a.Text == "" && a.Arguments != nil {
		a.Text = FlattenArguments(a.Arguments)
	}
	if a.Kind == "" {
		a.Kind = KindToolCall
	}
	summary := in.Summary
	if summary == "" {
		summary = ActionSummary(a)
	}

	e.lock.Lock()
	state := s.State
	fingerprints := s.State.Fingerprints
	e.lock.Unlock()

	lineage := MatchFingerprints(a.Text, fingerprints)
	for i := range lineage {
		lineage[i].OriginSummary = e.originSummary(state, lineage[i].OriginEventID)
	}
	class := Classify(a.Text)
	honeytoken := e.honeytokenHit(ctx, a.Text)

	e.refLock.Lock()
	policies := slices.Clone(e.policies)
	kills := slices.Clone(e.kills)
	settings := e.settings
	e.refLock.Unlock()

	now := e.now()
	input := PolicyInput{
		Subject: in.Subject, Action: a, Session: s, Settings: settings, Now: now,
		ActionLabels: class.Labels, Lineage: lineage, Honeytoken: honeytoken != nil,
	}

	// Kills are checked before policies so a killed session is denied even
	// if every policy is in shadow mode.
	killed := false
	for _, k := range kills {
		if k.Matches(s, now) {
			killed = true
			break
		}
	}
	input.Session.State.Killed = state.Killed || killed

	decision := e.evaluate(ctx, policies, input, honeytoken, in.Budget, start)
	decision.Labels = state.Labels
	decision.Risk = state.Risk
	decision.Lineage = lineage
	if killed {
		decision = Decision{
			Allow: false, Effect: EffectDeny, Killed: true, Labels: state.Labels, Risk: state.Risk,
			Reason: "the session was stopped with the kill switch", Matched: decision.Matched, Lineage: lineage,
		}
	}

	ev := Event{
		ID: NewID("evt"), SessionID: s.ID, Surface: a.Surface, Kind: a.Kind, At: now,
		UserID: in.Subject.UserID, Actor: in.Subject.Actor, ParentID: in.ParentID,
		Action: &a, Labels: class.Labels, Lineage: lineage, Decision: &decision, Ref: in.Ref,
		Summary: summary, Bytes: len(a.Text),
	}

	if decision.Effect == EffectRequireApproval && !decision.Shadow {
		approval, err := e.createApproval(ctx, s, &ev, decision, policies, settings)
		if err != nil {
			return nil, Decision{}, err
		}
		decision.ApprovalID = approval.ID
		ev.Decision = &decision
		if in.WaitForApproval {
			timeout := in.Budget
			if timeout <= 0 {
				timeout = approval.ExpiresAt.Sub(now)
			}
			if resolved, err := e.AwaitApproval(ctx, approval.ID, timeout); err == nil && resolved.Status == ApprovalApproved {
				decision.Allow = true
				decision.Reason = "approved by " + resolved.ResolvedBy + ": " + decision.Reason
				ev.Decision = &decision
			}
		}
	}

	e.lock.Lock()
	s.State.Events++
	if a.Kind == KindToolCall || a.Kind == KindShellExec {
		s.State.ToolCalls++
	}
	if !decision.Allow {
		s.State.Denied++
	}
	if decision.Allow && a.Sink.External {
		s.State.ExternalOut++
	}
	for _, m := range decision.Matched {
		if m.Shadow {
			s.State.Shadowed++
			break
		}
	}
	if killed {
		s.State.Killed = true
	}
	if s.State.Warrant != nil && s.State.Warrant.Active(now) && !s.State.Warrant.Covers(a) && !s.State.Warrant.Drifted {
		s.State.Warrant.Drifted = true
		s.State.Warrant.DriftSummary = summary
	}
	switch decision.Effect {
	case EffectQuarantine:
		s.State.Quarantined = true
	case EffectDownshift:
		s.State.Downshifted = true
	}
	s.State.LastSummary = summary
	s.State.Recompute()
	snapshot := *s
	e.lock.Unlock()

	if honeytoken != nil {
		e.tripHoneytoken(ctx, honeytoken)
	}
	if err := e.persist(ctx, &snapshot, ev); err != nil {
		return nil, Decision{}, err
	}
	return &ev, decision, nil
}

func (e *Engine) originSummary(state SessionState, eventID string) string {
	for _, origins := range state.Origins {
		for _, o := range origins {
			if o.EventID == eventID {
				return o.Summary
			}
		}
	}
	return ""
}

// evaluate runs the policies and resolves the strongest enforced effect.
func (e *Engine) evaluate(ctx context.Context, policies []Policy, in PolicyInput, honeytoken *Honeytoken, budget time.Duration, start time.Time) Decision {
	var matched []PolicyMatch
	var enforced, shadowed []*Policy
	for i := range policies {
		p := &policies[i]
		if !p.Enabled || !p.AppliesTo(in.Action.Surface) {
			continue
		}
		ok, err := EvalPolicy(p, in)
		if err != nil {
			e.logger.Warn("provenance: policy condition failed", "policy", p.Name, "error", err)
			continue
		}
		if !ok {
			continue
		}
		if p.Mode == ModeEnforce {
			enforced = append(enforced, p)
		} else {
			shadowed = append(shadowed, p)
		}
	}
	byStrength := func(ps []*Policy) {
		sort.SliceStable(ps, func(i, j int) bool {
			si, sj := effectSeverity(ps[i].Effect), effectSeverity(ps[j].Effect)
			if si != sj {
				return si > sj
			}
			return ps[i].Priority > ps[j].Priority
		})
	}
	byStrength(enforced)
	byStrength(shadowed)
	for _, p := range enforced {
		matched = append(matched, PolicyMatch{PolicyID: p.ID, PolicyName: p.Name, Effect: p.Effect})
	}
	for _, p := range shadowed {
		matched = append(matched, PolicyMatch{PolicyID: p.ID, PolicyName: p.Name, Effect: p.Effect, Shadow: true})
	}

	d := Decision{Allow: true, Effect: EffectAllow, Matched: matched, Honeytoken: honeytoken != nil}
	var winner *Policy
	shadow := false
	switch {
	case len(enforced) > 0:
		winner = enforced[0]
	case len(shadowed) > 0:
		winner = shadowed[0]
		shadow = true
	}
	if winner == nil {
		d.Reason = "no policy matched"
		return d
	}

	effect := winner.Effect
	rationale := ""
	if effect == EffectJudge {
		effect, rationale = e.judge(ctx, winner, in, budget, start, shadow)
	}
	d.Effect = effect
	d.PolicyID, d.PolicyName = winner.ID, winner.Name
	d.Shadow = shadow
	d.Reason = e.explain(winner, in, honeytoken, rationale)
	if shadow {
		d.Allow = true
		d.Reason = "would " + string(effect) + " (shadow): " + d.Reason
		return d
	}
	d.Allow = !effect.Blocks()
	if effect == EffectRedact {
		for _, sp := range Classify(in.Action.Text).Spans {
			d.Redactions = append(d.Redactions, sp.Span)
		}
	}
	return d
}

func effectSeverity(e Effect) int {
	if e == EffectJudge {
		return 5
	}
	s := e.Severity()
	if s >= 5 {
		return s + 1
	}
	return s
}

// judge asks the Supervisor to resolve a judge effect. Without a Supervisor,
// in shadow mode, or when the budget is too small, it degrades to
// require_approval.
func (e *Engine) judge(ctx context.Context, p *Policy, in PolicyInput, budget time.Duration, start time.Time, shadow bool) (Effect, string) {
	if shadow || e.supervisor == nil || !in.Settings.SupervisorEnabled {
		return EffectRequireApproval, ""
	}
	limit := in.Settings.SupervisorBudget
	if budget > 0 {
		remaining := budget - e.now().Sub(start)
		if remaining < 300*time.Millisecond {
			return EffectRequireApproval, "the Supervisor could not be consulted within the surface's time budget"
		}
		if limit <= 0 || remaining < limit {
			limit = remaining
		}
	}
	if limit <= 0 {
		limit = 4 * time.Second
	}
	jctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	recent, _, err := e.store.ListEvents(jctx, EventQuery{SessionID: in.Session.ID, Limit: recentEventsForJudge})
	if err != nil {
		recent = nil
	}
	out, err := e.supervisor.Judge(jctx, JudgeInput{
		Policy: *p, Subject: in.Subject, Session: in.Session, Action: in.Action,
		Narrative: in.Session.State.Explain(), Lineage: in.Lineage, Recent: recent, Budget: limit,
	})
	if err != nil {
		e.logger.Warn("provenance: supervisor judge failed", "policy", p.Name, "error", err)
		return EffectRequireApproval, "the Supervisor could not decide: " + err.Error()
	}
	switch out.Effect {
	case EffectAllow, EffectDeny, EffectRequireApproval:
	default:
		return EffectRequireApproval, "the Supervisor returned an unknown effect"
	}
	return out.Effect, fmt.Sprintf("Supervisor (%s, confidence %.0f%%): %s", cmpOr(out.Model, "model"), out.Confidence*100, out.Rationale)
}

// explain builds the human-readable reason for a decision.
func (e *Engine) explain(p *Policy, in PolicyInput, honeytoken *Honeytoken, rationale string) string {
	var b strings.Builder
	b.WriteString(p.Name)
	if p.Description != "" {
		b.WriteString(": ")
		b.WriteString(p.Description)
	}
	b.WriteString(" ")
	b.WriteString(strings.ToUpper(in.Session.State.Explain()[:1]) + in.Session.State.Explain()[1:])
	b.WriteString(". Now: ")
	b.WriteString(ActionSummary(in.Action))
	if in.Action.Sink.External && in.Action.Sink.Destination != "" {
		b.WriteString(" to external destination ")
		b.WriteString(in.Action.Sink.Destination)
	}
	b.WriteString(".")
	if len(in.Lineage) > 0 {
		b.WriteString(" Lineage: ")
		var parts []string
		for _, h := range in.Lineage {
			part := fmt.Sprintf("%d %s span(s)", h.Count, h.Label)
			if h.OriginSummary != "" {
				part += " from " + h.OriginSummary
			}
			parts = append(parts, part)
		}
		b.WriteString(strings.Join(parts, "; "))
		b.WriteString(".")
	}
	if honeytoken != nil {
		fmt.Fprintf(&b, " Honeytoken %q (%s) is in the outbound content.", honeytoken.Name, honeytoken.Kind)
	}
	if rationale != "" {
		b.WriteString(" ")
		b.WriteString(rationale)
	}
	return b.String()
}

// ActionSummary describes an action without its content.
func ActionSummary(a Action) string {
	switch a.Kind {
	case KindToolCall:
		if a.Server != "" {
			return "tools/call " + a.Server + "." + a.Tool
		}
		return "tools/call " + a.Tool
	case KindShellExec:
		cmd := a.Command
		if len(cmd) > 80 {
			cmd = cmd[:80] + "..."
		}
		return "shell: " + cmd
	case KindNetEgress:
		return strings.TrimSpace(a.Method + " " + a.Host + a.Path)
	case KindFileRead:
		return "read " + a.File
	case KindFileWrite:
		return "write " + a.File
	case KindLLMTurn:
		return "model turn " + a.Model
	}
	return string(a.Kind) + " " + a.Tool
}

func (e *Engine) createApproval(ctx context.Context, s *Session, ev *Event, d Decision, policies []Policy, settings Settings) (*Approval, error) {
	now := e.now()
	ttl := settings.DefaultApprovalTTL
	var approvers []string
	for _, p := range policies {
		if p.ID == d.PolicyID {
			if p.ApprovalTTL > 0 {
				ttl = p.ApprovalTTL
			}
			approvers = p.Approvers
		}
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	a := &Approval{
		ID: NewID("apr"), SessionID: s.ID, EventID: ev.ID, UserID: s.UserID, PolicyID: d.PolicyID, PolicyName: d.PolicyName,
		Action: *ev.Action, Reason: d.Reason, Approvers: approvers, CreatedAt: now, ExpiresAt: now.Add(ttl),
		Status: ApprovalPending, Risk: d.Risk, Labels: d.Labels,
	}
	a.Action.Text, a.Action.Arguments = "", nil
	if err := e.store.CreateApproval(ctx, a); err != nil {
		return nil, err
	}
	e.lock.Lock()
	s.State.Approvals++
	e.lock.Unlock()
	e.triageAsync(a, s)
	return a, nil
}

// triageAsync has the Supervisor pre-review an approval so the person who
// gets it sees a recommendation.
func (e *Engine) triageAsync(a *Approval, s *Session) {
	if e.supervisor == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if !e.Settings(ctx).SupervisorEnabled {
			return
		}
		recent, _, _ := e.store.ListEvents(ctx, EventQuery{SessionID: s.ID, Limit: recentEventsForJudge})
		t, err := e.supervisor.TriageApproval(ctx, *a, s, recent)
		if err != nil {
			e.logger.Debug("provenance: approval triage failed", "approval", a.ID, "error", err)
			return
		}
		current, err := e.store.GetApproval(ctx, a.ID)
		if err != nil {
			return
		}
		current.Triage = &t
		if err := e.store.UpdateApproval(ctx, current); err != nil {
			e.logger.Debug("provenance: storing triage", "approval", a.ID, "error", err)
		}
	}()
}

// ResolveApproval approves or denies a pending approval.
func (e *Engine) ResolveApproval(ctx context.Context, id string, approve bool, by, note string) (*Approval, error) {
	a, err := e.store.GetApproval(ctx, id)
	if err != nil {
		return nil, err
	}
	if a.Status != ApprovalPending {
		return a, fmt.Errorf("approval is already %s", a.Status)
	}
	now := e.now()
	if now.After(a.ExpiresAt) {
		a.Status = ApprovalExpired
	} else if approve {
		a.Status = ApprovalApproved
	} else {
		a.Status = ApprovalDenied
	}
	a.ResolvedBy, a.ResolvedAt, a.Note = by, &now, note
	if err := e.store.UpdateApproval(ctx, a); err != nil {
		return nil, err
	}
	ev := Event{
		ID: NewID("evt"), SessionID: a.SessionID, Surface: SurfaceLLM, Kind: KindApproval, At: now, UserID: a.UserID,
		ParentID: a.EventID, Summary: fmt.Sprintf("approval %s by %s", a.Status, by),
	}
	if s, err := e.session(ctx, a.SessionID); err == nil {
		ev.Surface = ""
		e.lock.Lock()
		s.State.Events++
		snapshot := *s
		e.lock.Unlock()
		if err := e.persist(ctx, &snapshot, ev); err != nil {
			return nil, err
		}
	}
	e.waitersLock.Lock()
	for _, ch := range e.waiters[id] {
		ch <- *a
	}
	delete(e.waiters, id)
	e.waitersLock.Unlock()
	return a, nil
}

// AwaitApproval blocks until the approval is resolved, the timeout passes,
// or the context ends. It returns the approval in its final state.
func (e *Engine) AwaitApproval(ctx context.Context, id string, timeout time.Duration) (*Approval, error) {
	ch := make(chan Approval, 1)
	e.waitersLock.Lock()
	e.waiters[id] = append(e.waiters[id], ch)
	e.waitersLock.Unlock()
	defer func() {
		e.waitersLock.Lock()
		ws := e.waiters[id]
		for i, w := range ws {
			if w == ch {
				e.waiters[id] = append(ws[:i], ws[i+1:]...)
				break
			}
		}
		e.waitersLock.Unlock()
	}()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(approvalPoll)
	defer ticker.Stop()
	for {
		select {
		case a := <-ch:
			return &a, nil
		case <-deadline.C:
			return e.store.GetApproval(ctx, id)
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			a, err := e.store.GetApproval(ctx, id)
			if err != nil {
				return nil, err
			}
			if a.Status != ApprovalPending {
				return a, nil
			}
		}
	}
}

// Record appends a generic event (an llm.turn, a delegation, a session.end)
// to a session.
func (e *Engine) Record(ctx context.Context, subject Subject, key SessionKey, ev Event) (*Event, error) {
	s, _, err := e.resolve(ctx, subject, key, ev.Surface)
	if err != nil {
		return nil, err
	}
	now := e.now()
	if ev.ID == "" {
		ev.ID = NewID("evt")
	}
	if ev.At.IsZero() {
		ev.At = now
	}
	ev.SessionID = s.ID
	if ev.UserID == "" {
		ev.UserID = subject.UserID
	}
	e.lock.Lock()
	s.State.Events++
	if ev.Summary != "" {
		s.State.LastSummary = ev.Summary
	}
	if ev.Kind == KindSessionEnd {
		s.EndedAt = &now
	}
	if ev.Kind == KindLLMTurn && s.Title == "" && ev.Summary != "" {
		s.Title = ev.Summary
	}
	snapshot := *s
	e.lock.Unlock()
	if err := e.persist(ctx, &snapshot, ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

// SetTitle sets a session's title (the declared task, or the Supervisor's
// one-line summary).
func (e *Engine) SetTitle(ctx context.Context, sessionID, title string) error {
	s, err := e.session(ctx, sessionID)
	if err != nil {
		return err
	}
	e.lock.Lock()
	s.Title = title
	snapshot := *s
	e.lock.Unlock()
	return e.store.PutSession(ctx, &snapshot)
}

// IssueWarrant binds a session to a declared task and the scopes it needs.
func (e *Engine) IssueWarrant(ctx context.Context, sessionID, task string, scopes []string, ttl time.Duration, delegation []string) (*Warrant, error) {
	s, err := e.session(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	now := e.now()
	if ttl <= 0 {
		ttl = 8 * time.Hour
	}
	w := &Warrant{
		ID: NewID("wrt"), SessionID: sessionID, Task: task, Scopes: scopes, Delegation: delegation,
		IssuedAt: now, ExpiresAt: now.Add(ttl),
	}
	e.lock.Lock()
	if parent := s.State.Warrant; parent != nil && parent.Active(now) {
		// Attenuation: a new warrant on a session with one can only narrow it.
		var narrowed []string
		for _, scope := range scopes {
			for _, ps := range parent.Scopes {
				if scopeWithin(scope, ps) {
					narrowed = append(narrowed, scope)
					break
				}
			}
		}
		w.Scopes = narrowed
		if w.ExpiresAt.After(parent.ExpiresAt) {
			w.ExpiresAt = parent.ExpiresAt
		}
		w.Delegation = append(slices.Clone(parent.Delegation), delegation...)
	}
	s.State.Warrant = w
	if s.Title == "" {
		s.Title = task
	}
	s.State.Recompute()
	snapshot := *s
	e.lock.Unlock()
	ev := Event{
		ID: NewID("evt"), SessionID: sessionID, Kind: KindDelegation, At: now, UserID: s.UserID,
		Summary: "warrant issued: " + task,
	}
	if err := e.persist(ctx, &snapshot, ev); err != nil {
		return nil, err
	}
	return w, nil
}

// scopeWithin reports whether scope is no broader than parent.
func scopeWithin(scope, parent string) bool {
	if parent == "*" || parent == scope {
		return true
	}
	pk, prest, _ := strings.Cut(parent, ":")
	sk, srest, _ := strings.Cut(scope, ":")
	if pk != sk {
		return false
	}
	if pk == "mcp" {
		pserver, ptool, _ := strings.Cut(prest, ":")
		sserver, stool, _ := strings.Cut(srest, ":")
		if ptool == "" {
			ptool = "*"
		}
		if stool == "" {
			stool = "*"
		}
		return globMatch(pserver, sserver) && globMatch(ptool, stool)
	}
	return prest == "" || globMatch(prest, srest)
}

// Kill activates a kill switch. Every action in its scope is denied on every
// surface from the next decision on.
func (e *Engine) Kill(ctx context.Context, k Kill) (*Kill, error) {
	if k.ID == "" {
		k.ID = NewID("kil")
	}
	if k.CreatedAt.IsZero() {
		k.CreatedAt = e.now()
	}
	switch k.Scope {
	case KillScopeOrg, KillScopeUser, KillScopeAgent, KillScopeSession:
	default:
		return nil, fmt.Errorf("unknown kill scope %q", k.Scope)
	}
	if k.Scope != KillScopeOrg && k.Target == "" {
		return nil, errors.New("kill target is required")
	}
	if err := e.store.PutKill(ctx, &k); err != nil {
		return nil, err
	}
	e.Invalidate()
	if k.Scope == KillScopeSession {
		if s, err := e.session(ctx, k.Target); err == nil {
			now := e.now()
			e.lock.Lock()
			s.State.Killed = true
			s.State.Recompute()
			snapshot := *s
			e.lock.Unlock()
			ev := Event{ID: NewID("evt"), SessionID: s.ID, Kind: KindKill, At: now, UserID: s.UserID, Summary: "session killed: " + k.Reason}
			if err := e.persist(ctx, &snapshot, ev); err != nil {
				return nil, err
			}
		}
	}
	return &k, nil
}

// Unkill removes a kill switch.
func (e *Engine) Unkill(ctx context.Context, id string) error {
	if err := e.store.DeleteKill(ctx, id); err != nil {
		return err
	}
	e.Invalidate()
	e.lock.Lock()
	for _, c := range e.sessions {
		c.session.State.Killed = false
	}
	e.lock.Unlock()
	return nil
}

// honeytokenHit reports which planted honeytoken, if any, appears in text.
func (e *Engine) honeytokenHit(ctx context.Context, text string) *Honeytoken {
	if text == "" {
		return nil
	}
	if err := e.loadRefs(ctx); err != nil {
		return nil
	}
	e.refLock.Lock()
	tokens := e.honeytokens
	e.refLock.Unlock()
	if len(tokens) == 0 {
		return nil
	}
	byHash := make(map[uint64]*Honeytoken, len(tokens))
	for i := range tokens {
		byHash[tokens[i].Hash] = &tokens[i]
	}
	class := Classify(text)
	for _, ids := range class.Identifiers {
		for _, id := range ids {
			if t, ok := byHash[hashExact(id)]; ok {
				return t
			}
		}
	}
	for _, tok := range strings.FieldsFunc(text, func(r rune) bool {
		return r == ' ' || r == '\n' || r == '\t' || r == '"' || r == '\'' || r == ',' || r == ';' || r == '=' || r == ':' || r == '&'
	}) {
		if len(tok) < 8 {
			continue
		}
		if t, ok := byHash[hashExact(tok)]; ok {
			return t
		}
	}
	return nil
}

func (e *Engine) tripHoneytoken(ctx context.Context, t *Honeytoken) {
	now := e.now()
	current, err := e.store.ListHoneytokens(ctx)
	if err != nil {
		return
	}
	for i := range current {
		if current[i].ID == t.ID {
			current[i].Tripped++
			current[i].LastTrip = &now
			if err := e.store.PutHoneytoken(ctx, &current[i]); err != nil {
				e.logger.Warn("provenance: recording honeytoken trip", "error", err)
			}
			e.Invalidate()
			return
		}
	}
}

// NewHoneytoken creates a honeytoken record for a value. The value is hashed;
// the caller plants it.
func NewHoneytoken(kind, name, value, createdBy string, now time.Time) Honeytoken {
	hint := value
	if len(hint) > 6 {
		hint = hint[:6] + "..."
	}
	return Honeytoken{ID: NewID("hny"), Kind: kind, Name: name, Hash: hashExact(value), Hint: hint, CreatedAt: now, CreatedBy: createdBy}
}

// persist writes the session state and appends an event.
func (e *Engine) persist(ctx context.Context, s *Session, ev Event) error {
	if err := e.store.PutSession(ctx, s); err != nil {
		return err
	}
	return e.store.AppendEvents(ctx, []Event{ev})
}

// EndSession closes a session.
func (e *Engine) EndSession(ctx context.Context, sessionID string, reason string) error {
	s, err := e.session(ctx, sessionID)
	if err != nil {
		return err
	}
	now := e.now()
	e.lock.Lock()
	s.EndedAt = &now
	s.State.Events++
	snapshot := *s
	e.lock.Unlock()
	return e.persist(ctx, &snapshot, Event{ID: NewID("evt"), SessionID: s.ID, Kind: KindSessionEnd, At: now, UserID: s.UserID, Summary: cmpOr(reason, "session ended")})
}

// CoverageOf computes coverage for sessions.
func CoverageOf(sessions []Session) Coverage {
	c := Coverage{Sessions: len(sessions), Surfaces: map[Surface]int{}}
	for _, s := range AllSurfaces {
		c.Surfaces[s] = 0
	}
	for _, s := range sessions {
		for _, surface := range s.State.Surfaces {
			c.Surfaces[surface]++
		}
		if len(s.State.Surfaces) <= 1 {
			c.Blind++
		}
	}
	return c
}
