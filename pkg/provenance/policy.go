package provenance

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

// Obot Policy is one CEL vocabulary over the subject, the action, the
// resource and the session, compiled to every enforcement point. A policy is a
// condition plus an effect; the ~9 policy types Obot used to have are sugar
// over it.
//
// Variables:
//
//	subject   {id, email, groups, agent, device, environment, api_key, delegation}
//	action    {surface, kind, tool, server, server_id, model, provider, host, method, path,
//	           command, file, capabilities, sink{external, destination, writes, exec},
//	           source{private, untrusted}, labels, lineage{hit, exact, count, labels},
//	           honeytoken, trifecta, text}
//	session   {id, labels, risk, events, tool_calls, denied, duration_seconds, surfaces,
//	           trifecta, warrant{present, task, scopes, drifted, covers}, killed}
//	org       {approved_destinations, trusted_hosts}
//
// Functions:
//
//	host_matches(host, patterns)  host matches any pattern ("*.example.com", "example.com")

const (
	ModeEnforce = "enforce"
	ModeShadow  = "shadow"

	// EffectJudge hands the action to the Supervisor model, which chooses
	// between allow, require_approval and deny with the whole causal chain in
	// view. It resolves to one of those effects.
	EffectJudge Effect = "judge"

	// SurfaceCopilot is Microsoft Copilot Studio agents, through its external
	// runtime monitoring hook.
	SurfaceCopilot Surface = "copilot"
)

var (
	policyEnvOnce sync.Once
	policyEnv     *cel.Env
	policyEnvErr  error

	policyProgramsLock sync.Mutex
	policyPrograms     = map[string]cel.Program{}
)

// Policy is one Obot Policy rule.
type Policy struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Condition is the CEL expression; it must evaluate to a boolean.
	Condition string `json:"condition"`
	Effect    Effect `json:"effect"`
	// Mode is enforce or shadow. A shadow policy records what it would have
	// done and never changes the outcome.
	Mode string `json:"mode"`
	// Priority orders policies with equal effect severity; higher wins.
	Priority int `json:"priority"`
	// Surfaces limits the policy to some surfaces; empty means all.
	Surfaces []Surface `json:"surfaces,omitempty"`
	Enabled  bool      `json:"enabled"`
	// Guidance is extra plain-language guidance for the Supervisor when the
	// effect is judge ("err on the side of allowing code pushes to the
	// user's own fork").
	Guidance string `json:"guidance,omitempty"`
	// Approvers names who can resolve an approval the policy requires:
	// "admin", "manager", "group:<name>", "user:<id>". Empty means admin.
	Approvers []string `json:"approvers,omitempty"`
	// ApprovalTTL is how long an approval request stays open.
	ApprovalTTL time.Duration `json:"approvalTTL,omitempty"`
	// BuiltIn marks a policy Obot seeded. Built-in policies can be disabled or
	// switched to enforce but not deleted.
	BuiltIn   bool      `json:"builtIn,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	// Source is the plain-language rule the policy was drafted from, when the
	// Supervisor wrote it.
	Source string `json:"source,omitempty"`
}

// Settings are organization-wide inputs to decisions.
type Settings struct {
	// ApprovedDestinations are host patterns that are not external
	// ("*.corp.example.com", "api.github.com").
	ApprovedDestinations []string `json:"approvedDestinations,omitempty"`
	// TrustedHosts are hosts whose content is first-party, not untrusted.
	TrustedHosts []string `json:"trustedHosts,omitempty"`
	// SupervisorEnabled turns on the Supervisor model for judge effects,
	// approval triage and policy drafting.
	SupervisorEnabled bool `json:"supervisorEnabled"`
	// SupervisorModel is the model the Supervisor calls; empty means the
	// default model alias.
	SupervisorModel string `json:"supervisorModel,omitempty"`
	// SupervisorBudget bounds a judge call on the hot path.
	SupervisorBudget time.Duration `json:"supervisorBudget,omitempty"`
	// DefaultApprovalTTL applies when a policy sets none.
	DefaultApprovalTTL time.Duration `json:"defaultApprovalTTL,omitempty"`
	// RetentionDays is how long sessions and events are kept.
	RetentionDays int `json:"retentionDays,omitempty"`
}

// PolicyInput is everything a condition can see.
type PolicyInput struct {
	Subject  Subject
	Action   Action
	Session  *Session
	Settings Settings
	Now      time.Time
	// ActionLabels are the sensitivity labels found in the action's content.
	ActionLabels Labels
	Lineage      []LineageHit
	Honeytoken   bool
}

func init() {
	AllSurfaces = append(AllSurfaces, SurfaceCopilot)
}

// DefaultSettings are used until an administrator changes them.
func DefaultSettings() Settings {
	return Settings{
		SupervisorEnabled:  true,
		SupervisorBudget:   4 * time.Second,
		DefaultApprovalTTL: 10 * time.Minute,
		RetentionDays:      90,
	}
}

// Validate checks a policy's fields and compiles its condition.
func (p *Policy) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("policy name is required")
	}
	if p.Effect != EffectJudge && !ValidEffect(p.Effect) {
		return fmt.Errorf("unknown effect %q", p.Effect)
	}
	switch p.Mode {
	case ModeEnforce, ModeShadow:
	case "":
		p.Mode = ModeShadow
	default:
		return fmt.Errorf("unknown mode %q", p.Mode)
	}
	for _, s := range p.Surfaces {
		if !ValidSurface(s) {
			return fmt.Errorf("unknown surface %q", s)
		}
	}
	if strings.TrimSpace(p.Condition) == "" {
		return errors.New("policy condition is required")
	}
	if _, err := CompilePolicy(p.Condition); err != nil {
		return err
	}
	// Every variable is always present, so a smoke evaluation on an empty
	// input catches conditions that don't produce a boolean.
	_, err := EvalPolicy(p, PolicyInput{Session: &Session{}, Now: time.Now()})
	return err
}

// ValidSurface reports whether s is a known surface.
func ValidSurface(s Surface) bool {
	return s == SurfaceLLM || s == SurfaceMCP || s == SurfaceDevice || s == SurfaceEgress || s == SurfaceCopilot
}

// AppliesTo reports whether the policy covers a surface.
func (p *Policy) AppliesTo(s Surface) bool {
	if len(p.Surfaces) == 0 {
		return true
	}
	return slices.Contains(p.Surfaces, s)
}

func celPolicyEnv() (*cel.Env, error) {
	policyEnvOnce.Do(func() {
		policyEnv, policyEnvErr = cel.NewEnv(
			cel.Variable("subject", cel.MapType(cel.StringType, cel.DynType)),
			cel.Variable("action", cel.MapType(cel.StringType, cel.DynType)),
			cel.Variable("session", cel.MapType(cel.StringType, cel.DynType)),
			cel.Variable("org", cel.MapType(cel.StringType, cel.DynType)),
			cel.Function("host_matches",
				cel.Overload("host_matches_string_list", []*cel.Type{cel.StringType, cel.ListType(cel.StringType)}, cel.BoolType,
					cel.BinaryBinding(func(host, patterns ref.Val) ref.Val {
						h, ok := host.Value().(string)
						if !ok {
							return types.Bool(false)
						}
						list, ok := patterns.(interface{ Value() any })
						if !ok {
							return types.Bool(false)
						}
						var ps []string
						switch v := list.Value().(type) {
						case []string:
							ps = v
						case []any:
							for _, x := range v {
								if s, ok := x.(string); ok {
									ps = append(ps, s)
								}
							}
						case []ref.Val:
							for _, x := range v {
								if s, ok := x.Value().(string); ok {
									ps = append(ps, s)
								}
							}
						}
						return types.Bool(HostMatches(h, ps))
					}),
				),
			),
		)
	})
	return policyEnv, policyEnvErr
}

// CompilePolicy compiles a policy condition, caching the program.
func CompilePolicy(expr string) (cel.Program, error) {
	policyProgramsLock.Lock()
	defer policyProgramsLock.Unlock()
	if p, ok := policyPrograms[expr]; ok {
		return p, nil
	}
	env, err := celPolicyEnv()
	if err != nil {
		return nil, err
	}
	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}
	// Fields of the map-typed variables (action.honeytoken, session.killed)
	// are dyn at compile time; the boolean requirement is enforced at
	// evaluation for them.
	if out := ast.OutputType(); out != cel.BoolType && out != cel.DynType {
		return nil, fmt.Errorf("condition must evaluate to a boolean, not %s", out)
	}
	p, err := env.Program(ast)
	if err != nil {
		return nil, err
	}
	policyPrograms[expr] = p
	return p, nil
}

// HostMatches reports whether host matches any pattern. A pattern is an exact
// host or a single-label wildcard "*.example.com" (which also matches
// "example.com"), or "*".
func HostMatches(host string, patterns []string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if h, _, ok := strings.Cut(host, ":"); ok {
		host = h
	}
	for _, p := range patterns {
		p = strings.ToLower(strings.TrimSpace(p))
		switch {
		case p == "":
			continue
		case p == "*":
			return true
		case strings.HasPrefix(p, "*."):
			base := p[2:]
			if host == base || strings.HasSuffix(host, "."+base) {
				return true
			}
		case p == host:
			return true
		}
	}
	return false
}

// Vars renders the input as CEL variables. Every declared variable is always
// present so a condition never fails on a missing top-level name.
func (in PolicyInput) Vars() map[string]any {
	a := in.Action
	lineageLabels := Labels{}
	lineageCount := 0
	exact := false
	for _, h := range in.Lineage {
		lineageLabels = lineageLabels.Add(h.Label)
		lineageCount += h.Count
		exact = exact || h.Exact
	}
	actionVars := map[string]any{
		"surface":      string(a.Surface),
		"kind":         string(a.Kind),
		"tool":         a.Tool,
		"server":       a.Server,
		"server_id":    a.ServerID,
		"model":        a.Model,
		"provider":     a.Provider,
		"host":         a.Host,
		"method":       a.Method,
		"path":         a.Path,
		"command":      a.Command,
		"file":         a.File,
		"capabilities": a.Capabilities.Anys(),
		"sink": map[string]any{
			"external":    a.Sink.External,
			"destination": a.Sink.Destination,
			"writes":      a.Sink.Writes,
			"exec":        a.Sink.Exec,
		},
		"source": map[string]any{
			"private":   a.Source.Private,
			"untrusted": a.Source.Untrusted,
		},
		"labels": in.ActionLabels.Anys(),
		"lineage": map[string]any{
			"hit":    len(in.Lineage) > 0,
			"exact":  exact,
			"count":  lineageCount,
			"labels": lineageLabels.Anys(),
		},
		"honeytoken": in.Honeytoken,
		"text":       a.Text,
	}

	st := SessionState{}
	sessionID := ""
	var started time.Time
	if in.Session != nil {
		st = in.Session.State
		sessionID = in.Session.ID
		started = in.Session.StartedAt
	}
	actionVars["trifecta"] = st.Trifecta() && a.Sink.External
	surfaces := make([]any, 0, len(st.Surfaces))
	for _, s := range st.Surfaces {
		surfaces = append(surfaces, string(s))
	}
	warrant := map[string]any{"present": false, "task": "", "scopes": []any{}, "drifted": false, "covers": true}
	if st.Warrant != nil {
		scopes := make([]any, 0, len(st.Warrant.Scopes))
		for _, s := range st.Warrant.Scopes {
			scopes = append(scopes, s)
		}
		warrant = map[string]any{
			"present": st.Warrant.Active(in.Now),
			"task":    st.Warrant.Task,
			"scopes":  scopes,
			"drifted": st.Warrant.Drifted,
			"covers":  st.Warrant.Covers(a),
		}
	}
	duration := 0.0
	if !started.IsZero() {
		duration = in.Now.Sub(started).Seconds()
	}
	sessionVars := map[string]any{
		"id":               sessionID,
		"labels":           st.Labels.Anys(),
		"risk":             st.Risk,
		"events":           st.Events,
		"tool_calls":       st.ToolCalls,
		"denied":           st.Denied,
		"duration_seconds": duration,
		"surfaces":         surfaces,
		"trifecta":         st.Trifecta(),
		"warrant":          warrant,
		"killed":           st.Killed,
	}

	groups := make([]any, 0, len(in.Subject.Groups))
	for _, g := range in.Subject.Groups {
		groups = append(groups, g)
	}
	delegation := make([]any, 0, len(in.Subject.Actor.Delegation))
	for _, d := range in.Subject.Actor.Delegation {
		delegation = append(delegation, d)
	}
	subjectVars := map[string]any{
		"id":          in.Subject.UserID,
		"email":       in.Subject.Email,
		"groups":      groups,
		"agent":       in.Subject.Actor.Agent,
		"device":      in.Subject.Actor.DeviceID,
		"environment": in.Subject.Actor.EnvironmentID,
		"api_key":     in.Subject.Actor.APIKeyID,
		"delegation":  delegation,
	}
	approved := make([]any, 0, len(in.Settings.ApprovedDestinations))
	for _, d := range in.Settings.ApprovedDestinations {
		approved = append(approved, d)
	}
	trusted := make([]any, 0, len(in.Settings.TrustedHosts))
	for _, d := range in.Settings.TrustedHosts {
		trusted = append(trusted, d)
	}
	return map[string]any{
		"subject": subjectVars,
		"action":  actionVars,
		"session": sessionVars,
		"org": map[string]any{
			"approved_destinations": approved,
			"trusted_hosts":         trusted,
		},
	}
}

// EvalPolicy reports whether a policy's condition holds for the input.
func EvalPolicy(p *Policy, in PolicyInput) (bool, error) {
	prog, err := CompilePolicy(p.Condition)
	if err != nil {
		return false, err
	}
	out, _, err := prog.Eval(in.Vars())
	if err != nil {
		return false, err
	}
	b, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("condition evaluated to %T, not a boolean", out.Value())
	}
	return b, nil
}

// BuiltInPolicies are seeded when the policy store is empty. They start in
// shadow mode, except the honeytoken rule, which has no false positives.
func BuiltInPolicies(now time.Time) []Policy {
	mk := func(id, name, desc, cond string, effect Effect, mode string, prio int, guidance string) Policy {
		return Policy{
			ID: id, Name: name, Description: desc, Condition: cond, Effect: effect, Mode: mode,
			Priority: prio, Enabled: true, BuiltIn: true, Guidance: guidance, CreatedAt: now, UpdatedAt: now,
		}
	}
	return []Policy{
		mk("builtin-honeytoken", "Honeytoken exfiltration",
			"A planted canary credential or record is leaving the session. This is definitive exfiltration evidence.",
			`action.honeytoken`, EffectDeny, ModeEnforce, 1000, ""),
		mk("builtin-lethal-trifecta", "Lethal trifecta",
			"The session has read untrusted content and confidential data, and this action sends data to an external destination.",
			`action.trifecta`, EffectRequireApproval, ModeShadow, 900, ""),
		mk("builtin-secrets-out", "Secrets never leave",
			"A secret (API key, token, private key) that entered the session is in an outbound action.",
			`"secret" in action.lineage.labels && action.sink.external`, EffectDeny, ModeShadow, 950, ""),
		mk("builtin-pii-lineage", "PII lineage to external destination",
			"PII read earlier in the session appears in an action that writes outside the organization.",
			`"pii" in action.lineage.labels && action.sink.external`, EffectRequireApproval, ModeShadow, 800, ""),
		mk("builtin-untrusted-destructive", "Destructive action after untrusted content",
			"The session has read untrusted content and now wants to run a destructive or executing tool. The Supervisor decides.",
			`"untrusted" in session.labels && ("destructive" in action.capabilities || action.sink.exec)`, EffectJudge, ModeShadow, 700,
			"Allow routine development commands (tests, builds, git on the user's own branches). Escalate deletions of shared data and anything that sends data out."),
		mk("builtin-warrant-drift", "Outside the declared task",
			"The action is outside the scopes of the session's warrant. The Supervisor decides whether it fits the declared task.",
			`session.warrant.present && !session.warrant.covers`, EffectJudge, ModeShadow, 600,
			"Judge whether the action plausibly serves the declared task. Reading documentation usually does; reading email or CRM data usually does not."),
		mk("builtin-killed", "Killed session",
			"The session, user or organization was killed with the kill switch.",
			`session.killed`, EffectDeny, ModeEnforce, 2000, ""),
	}
}
