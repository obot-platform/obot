package guardrails

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"time"

	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/messagepolicy"
)

const (
	// cacheTTL bounds how long policy and guardrail changes take to apply on other replicas.
	cacheTTL = 5 * time.Second
)

var (
	_ Store = (*gateway.Client)(nil)
)

// Identity is who a request is made for.
type Identity struct {
	UserID string
	Groups []string
	// ProfileIDs are the gateway profiles that apply to the caller. LLM policies apply through
	// them.
	ProfileIDs []string
}

// Request is content to evaluate.
type Request struct {
	Target   string
	Stage    string
	Identity Identity
	// Vars are the target's condition variables (model, provider, headers, mcp_tool, ...).
	Vars     map[string]any
	Segments []Segment
	History  []Segment
	// Delivered reports that the content has already reached the client, as with a stream
	// that couldn't be held. Findings that would block or redact then end the response and
	// are recorded as errors, since the content can no longer be withheld.
	Delivered bool
}

// Decision is the combined outcome of every applicable policy.
type Decision struct {
	Blocked bool
	// Message explains a block to the caller.
	Message string
	// Segments is the request's segments after redaction.
	Segments []Segment
	// Redacted reports whether any segment changed.
	Redacted bool
	// Detections are what each policy did to the request, in the order policies ran: one for
	// every guardrail that found something or failed, and one, without a guardrail, for a
	// fail-closed policy whose condition failed. They never include the content.
	Detections []Detection
}

// Detection is one policy's outcome on a request: which guardrail found something, the
// categories it found, and what the policy did about it.
type Detection struct {
	Stage         string
	PolicyID      string
	PolicyName    string
	GuardrailName string
	GuardrailType string
	// Outcome is blocked, redacted, logged or error.
	Outcome    string
	Categories []string
	// Error is why the guardrail could not be evaluated, for an error outcome.
	Error string
}

// Plan summarizes the policies that apply to a request before it is evaluated, which callers
// use to decide how much of a streamed response to hold back.
type Plan struct {
	policies []*compiledPolicy
	// broken holds fail-closed policies whose condition could not be evaluated. They block.
	broken []brokenPolicy
}

// brokenPolicy is a fail-closed policy whose condition could not be evaluated, and why.
type brokenPolicy struct {
	*compiledPolicy
	err error
}

type compiledGuardrail struct {
	guardrail types.Guardrail
	checker   Checker
	redacts   bool
	err       error
}

type compiledPolicy struct {
	policy     types.GuardrailPolicy
	guardrails []*compiledGuardrail
	// allRedact reports whether every guardrail can locate findings, so redaction never has
	// to fall back to blocking.
	allRedact bool
}

// checkKey identifies one guardrail call within an Evaluate call. Every policy's checker input
// is derived from the decision's current segments, split at offset into what is checked and
// what is added to the history; generation counts the redactions applied to those segments so
// far. Equal keys therefore mean identical input, and the guardrail's result can be reused.
type checkKey struct {
	guardrailID string
	offset      int
	generation  int
}

type checkResult struct {
	result Result
	err    error
}

// Store is where policies and guardrails live. *gateway.Client implements it.
type Store interface {
	ListGuardrailPolicies(ctx context.Context) ([]types.GuardrailPolicy, error)
	ListGuardrails(ctx context.Context) ([]types.Guardrail, error)
	GuardrailSecrets(ctx context.Context, id string) (map[string]string, error)
}

// Engine evaluates guardrail policies.
type Engine struct {
	client Store
	deps   Deps

	lock       sync.Mutex
	loadedAt   time.Time
	policies   []*compiledPolicy
	guardrails map[string]*compiledGuardrail

	// conditionErrors throttles warnings for policies whose condition fails at runtime, which
	// would otherwise log one per request.
	conditionErrorsLock sync.Mutex
	conditionErrors     map[string]time.Time
}

// Empty reports whether no policy applies.
func (p Plan) Empty() bool { return len(p.policies) == 0 && len(p.broken) == 0 }

// CanBlock reports whether any policy may block, which requires holding the whole response.
func (p Plan) CanBlock() bool {
	return len(p.broken) > 0 || slices.ContainsFunc(p.policies, func(r *compiledPolicy) bool {
		return r.policy.Action == types.GuardrailActionBlock || (r.policy.Action == types.GuardrailActionRedact && !r.allRedact) || r.policy.FailClosed
	})
}

// Modifies reports whether any policy may change or stop the response.
func (p Plan) Modifies() bool {
	return len(p.broken) > 0 || slices.ContainsFunc(p.policies, func(r *compiledPolicy) bool {
		return r.policy.Action != types.GuardrailActionLog || r.policy.FailClosed
	})
}

// New creates an engine. judge may be nil, in which case LLM-judge guardrails fail.
func New(client Store, judge *messagepolicy.Helper) *Engine {
	return &Engine{
		client: client,
		deps: Deps{
			Judge:      judge,
			HTTPClient: newHTTPClient(),
		},
	}
}

// Invalidate makes the next evaluation reload policies and guardrails.
func (e *Engine) Invalidate() {
	e.lock.Lock()
	e.loadedAt = time.Time{}
	e.lock.Unlock()
}

func (e *Engine) load(ctx context.Context) ([]*compiledPolicy, error) {
	policies, _, err := e.loadAll(ctx)
	return policies, err
}

// loadAll returns the enabled policies and every guardrail, by ID.
func (e *Engine) loadAll(ctx context.Context) ([]*compiledPolicy, map[string]*compiledGuardrail, error) {
	e.lock.Lock()
	defer e.lock.Unlock()
	if time.Since(e.loadedAt) < cacheTTL {
		return e.policies, e.guardrails, nil
	}

	policies, err := e.client.ListGuardrailPolicies(ctx)
	if err != nil {
		return nil, nil, err
	}
	guardrails, err := e.client.ListGuardrails(ctx)
	if err != nil {
		return nil, nil, err
	}

	byID := map[string]*compiledGuardrail{}
	for _, p := range guardrails {
		cp := &compiledGuardrail{guardrail: p}
		info, ok := Provider(p.Type)
		cp.redacts = ok && info.Redacts
		var config map[string]any
		if len(p.Config) > 0 {
			if err := json.Unmarshal(p.Config, &config); err != nil {
				cp.err = fmt.Errorf("invalid configuration: %w", err)
			}
		}
		if cp.err == nil {
			secrets, err := e.client.GuardrailSecrets(ctx, p.ID)
			if err != nil {
				cp.err = err
			} else if cp.checker, err = newChecker(p.Type, config, secrets, e.deps); err != nil {
				cp.err = err
			}
		}
		byID[p.ID] = cp
	}

	compiled := make([]*compiledPolicy, 0, len(policies))
	for _, r := range policies {
		if !r.Enabled {
			continue
		}
		cr := &compiledPolicy{policy: r, allRedact: true}
		for _, id := range r.GuardrailIDs {
			p, ok := byID[id]
			if !ok {
				// A guardrail deleted while a policy still names it fails like a guardrail error, so
				// the policy keeps applying and follows its fail-open or fail-closed setting, as
				// guardrail filters do.
				p = &compiledGuardrail{guardrail: types.Guardrail{ID: id, Name: id}, err: fmt.Errorf("guardrail %s does not exist", id)}
			}
			cr.guardrails = append(cr.guardrails, p)
			cr.allRedact = cr.allRedact && p.redacts
		}
		if len(cr.guardrails) > 0 {
			compiled = append(compiled, cr)
		}
	}
	e.policies = compiled
	e.guardrails = byID
	e.loadedAt = time.Now()
	return compiled, byID, nil
}

func stageMatches(applyTo, stage string) bool {
	return applyTo == types.GuardrailApplyBoth || applyTo == stage
}

// appliesTo reports whether a policy applies to the caller. An LLM policy applies through its
// gateway profile, and one without a profile applies to no one. An MCP guardrail filter's
// policy applies to its subjects.
func appliesTo(policy types.GuardrailPolicy, identity Identity) bool {
	if policy.Target == types.GuardrailTargetLLM {
		return policy.ProfileID != "" && slices.Contains(identity.ProfileIDs, policy.ProfileID)
	}
	return subjectsMatch(policy.Subjects, identity)
}

func subjectsMatch(subjects []types.GuardrailPolicySubject, identity Identity) bool {
	if len(subjects) == 0 {
		return true
	}
	return slices.ContainsFunc(subjects, func(s types.GuardrailPolicySubject) bool {
		switch s.Type {
		case types.LLMGatewayBudgetSubjectEveryone, "selector":
			return true
		case types.LLMGatewayBudgetSubjectUser:
			return s.ID == identity.UserID
		case types.LLMGatewayBudgetSubjectGroup:
			return slices.Contains(identity.Groups, s.ID)
		}
		return false
	})
}

// Plan returns the policies that apply to a request's target, stage, caller and variables.
// Sampling is decided here, once per request and stage.
func (e *Engine) Plan(ctx context.Context, target, stage string, identity Identity, vars map[string]any) (Plan, error) {
	policies, err := e.load(ctx)
	if err != nil {
		return Plan{}, err
	}
	var plan Plan
	for _, r := range policies {
		e.addToPlan(&plan, r, target, stage, identity, vars)
	}
	return plan, nil
}

// addToPlan adds a policy to a plan when it applies to the target, stage, caller and
// variables, and is sampled.
func (e *Engine) addToPlan(plan *Plan, r *compiledPolicy, target, stage string, identity Identity, vars map[string]any) {
	if r.policy.Target != target || !stageMatches(r.policy.ApplyTo, stage) || !appliesTo(r.policy, identity) {
		return
	}
	ok, err := evalCondition(target, r.policy.Condition, identity, vars)
	if err != nil {
		// A condition can fail at runtime, e.g. headers["x"] when the header is absent.
		// Fail-closed policies block; others are skipped. Either way the failure is logged.
		e.conditionFailed(r, err)
		if r.policy.FailClosed {
			plan.broken = append(plan.broken, brokenPolicy{compiledPolicy: r, err: err})
		}
		return
	}
	if !ok {
		return
	}
	if rate := r.policy.SamplingRate; rate > 0 && rate < 100 && rand.IntN(100) >= rate {
		return
	}
	plan.policies = append(plan.policies, r)
}

// Evaluate checks a request against a plan's policies. Policies run in order; a block stops
// evaluation, and each redaction is applied before the next policy sees the content. A
// guardrail used by several policies is called once for the same content, and each policy
// applies the result, or the error, according to its own settings.
func (e *Engine) Evaluate(ctx context.Context, plan Plan, req Request) Decision {
	d := Decision{Segments: req.Segments}
	var (
		generation int
		checked    = map[checkKey]checkResult{}
	)
	if len(plan.broken) > 0 {
		d.Blocked = true
		broken := plan.broken[0]
		d.Message = fmt.Sprintf("%s blocked: the condition of the %q guardrail could not be evaluated.", blockedContent(req.Stage), broken.policy.Name)
		d.Detections = append(d.Detections, Detection{
			Stage: req.Stage, PolicyID: broken.policy.ID, PolicyName: broken.policy.Name, Outcome: "error",
			Error: "the condition could not be evaluated: " + broken.err.Error(),
		})
		return d
	}
	for _, r := range plan.policies {
		segments := d.Segments
		offset := 0
		// Input scope only applies to conversations; an MCP tool call has no earlier turns.
		if req.Target == types.GuardrailTargetLLM && req.Stage == StageInput && r.policy.InputScope == types.GuardrailScopeLatest && r.policy.Action != types.GuardrailActionRedact {
			offset = latestStart(segments)
			segments = segments[offset:]
		}
		if len(segments) == 0 {
			continue
		}

		for _, p := range r.guardrails {
			if p.err != nil {
				if e.fail(ctx, r, p, req, p.err, &d) {
					return d
				}
				continue
			}
			key := checkKey{guardrailID: p.guardrail.ID, offset: offset, generation: generation}
			cached, ok := checked[key]
			if !ok {
				cached.result, cached.err = p.checker.Check(ctx, CheckInput{
					Target:   req.Target,
					Stage:    req.Stage,
					Segments: segments,
					History:  append(slices.Clip(req.History), d.Segments[:offset]...),
				})
				checked[key] = cached
			}
			result, err := cached.result, cached.err
			if err != nil {
				if e.fail(ctx, r, p, req, err, &d) {
					return d
				}
				continue
			}
			if len(result.Findings) == 0 && len(result.Replacements) == 0 {
				continue
			}

			outcome := "logged"
			switch r.policy.Action {
			case types.GuardrailActionBlock:
				outcome = "blocked"
			case types.GuardrailActionRedact:
				redacted, ok := redactResult(segments, result)
				if !ok {
					// The checker can't say where the problem is, so it can't be removed.
					outcome = "blocked"
					break
				}
				if req.Delivered {
					outcome = "blocked"
					break
				}
				outcome = "redacted"
				d.Segments = append(slices.Clip(d.Segments[:offset]), redacted...)
				segments = redacted
				d.Redacted = true
				generation++
			}
			if outcome == "blocked" && req.Delivered {
				// Too late to withhold anything: end the response and say so.
				e.record(r, p, req, "error", result.Findings, "the content was already delivered because the response could not be held", &d)
				d.Blocked = true
				d.Message = blockMessage(r, result.Findings)
				return d
			}
			e.record(r, p, req, outcome, result.Findings, "", &d)
			if outcome == "blocked" {
				d.Blocked = true
				d.Message = blockMessage(r, result.Findings)
				return d
			}
		}
	}
	return d
}

// conditionFailed logs a policy whose condition failed to evaluate, at most once a minute
// per policy.
func (e *Engine) conditionFailed(r *compiledPolicy, err error) {
	e.conditionErrorsLock.Lock()
	last, seen := e.conditionErrors[r.policy.ID]
	if seen && time.Since(last) < time.Minute {
		e.conditionErrorsLock.Unlock()
		return
	}
	if e.conditionErrors == nil {
		e.conditionErrors = map[string]time.Time{}
	}
	e.conditionErrors[r.policy.ID] = time.Now()
	e.conditionErrorsLock.Unlock()

	slog.Warn("guardrail policy condition failed to evaluate", "policy", r.policy.Name, "failClosed", r.policy.FailClosed, "error", err)
}

// fail handles a guardrail that couldn't be evaluated. It reports whether the request is
// blocked.
func (e *Engine) fail(ctx context.Context, r *compiledPolicy, p *compiledGuardrail, req Request, err error, d *Decision) bool {
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	slog.Warn("guardrail failed", "policy", r.policy.Name, "guardrail", p.guardrail.Name, "error", err)
	e.record(r, p, req, "error", nil, err.Error(), d)
	if !r.policy.FailClosed {
		return false
	}
	d.Blocked = true
	d.Message = fmt.Sprintf("%s blocked: the %q guardrail could not be evaluated.", blockedContent(req.Stage), r.policy.Name)
	return true
}

// blockedContent names what a stage blocks: the request on input, the response on output.
func blockedContent(stage string) string {
	if stage == StageOutput {
		return "Response"
	}
	return "Request"
}

func redactResult(segments []Segment, result Result) ([]Segment, bool) {
	out := slices.Clone(segments)
	for i, text := range result.Replacements {
		if i >= 0 && i < len(out) {
			out[i].Text = text
		}
	}
	for i := range result.Replacements {
		if i < 0 || i >= len(out) {
			return nil, false
		}
	}
	var spans []Span
	for _, f := range result.Findings {
		if len(f.Spans) == 0 {
			if len(result.Replacements) > 0 {
				continue
			}
			return nil, false
		}
		for _, s := range f.Spans {
			// A span that doesn't fit the text would redact nothing; block instead.
			if s.Segment < 0 || s.Segment >= len(out) || s.Start < 0 || s.End <= s.Start || s.End > len(out[s.Segment].Text) {
				return nil, false
			}
		}
		spans = append(spans, f.Spans...)
	}
	return ApplySpans(out, spans), true
}

// latestStart returns the index of the first segment after the last assistant message or
// tool call, i.e. what the caller added since the model last responded.
func latestStart(segments []Segment) int {
	for i, s := range slices.Backward(segments) {
		if s.Role == RoleAssistant || s.Role == RoleToolCall {
			return i + 1
		}
	}
	return 0
}

func blockMessage(r *compiledPolicy, findings []Finding) string {
	if r.policy.BlockMessage != "" {
		return r.policy.BlockMessage
	}
	var reasons []string
	for _, f := range findings {
		reason := cmp.Or(f.Explanation, f.Message, f.Category)
		if reason != "" && !slices.Contains(reasons, reason) {
			reasons = append(reasons, reason)
		}
	}
	msg := fmt.Sprintf("Blocked by the %q guardrail", r.policy.Name)
	if len(reasons) > 0 {
		msg += ": " + strings.Join(reasons, "; ")
	}
	// Reasons, such as a judge's explanation, may already end in punctuation.
	if strings.HasSuffix(msg, ".") || strings.HasSuffix(msg, "!") || strings.HasSuffix(msg, "?") {
		return msg
	}
	return msg + "."
}

// record records a policy's outcome as a detection on the decision.
func (e *Engine) record(r *compiledPolicy, p *compiledGuardrail, req Request, outcome string, findings []Finding, errMsg string, d *Decision) {
	var categories []string
	for _, f := range findings {
		if f.Category != "" && !slices.Contains(categories, f.Category) {
			categories = append(categories, f.Category)
		}
	}
	d.Detections = append(d.Detections, Detection{
		Stage:         req.Stage,
		PolicyID:      r.policy.ID,
		PolicyName:    r.policy.Name,
		GuardrailName: p.guardrail.Name,
		GuardrailType: p.guardrail.Type,
		Outcome:       outcome,
		Categories:    categories,
		Error:         errMsg,
	})
}

// FieldOptions loads a field's choices from the service a guardrail's settings point at.
func (e *Engine) FieldOptions(ctx context.Context, typ, field string, config map[string]any, secrets map[string]string) ([]string, error) {
	loader, ok := optionsLoader(typ, field)
	if !ok {
		return nil, fmt.Errorf("%s guardrails can't load choices for %q", typ, field)
	}
	return loader(ctx, config, secrets, e.deps)
}

// TestGuardrail runs a guardrail's checker against sample text without recording anything, so
// admins can try a guardrail before relying on it.
func (e *Engine) TestGuardrail(ctx context.Context, guardrail types.Guardrail, secrets map[string]string, target, stage, text string) (Result, []Segment, error) {
	var config map[string]any
	if len(guardrail.Config) > 0 {
		if err := json.Unmarshal(guardrail.Config, &config); err != nil {
			return Result{}, nil, fmt.Errorf("invalid configuration: %w", err)
		}
	}
	checker, err := newChecker(guardrail.Type, config, secrets, e.deps)
	if err != nil {
		return Result{}, nil, err
	}
	role := RoleUser
	switch {
	case target == types.GuardrailTargetMCP && stage == StageOutput:
		role = RoleToolResult
	case target == types.GuardrailTargetMCP:
		role = RoleToolCall
	case stage == StageOutput:
		role = RoleAssistant
	}
	segments := []Segment{{Role: role, Text: text, Name: "test"}}
	result, err := checker.Check(ctx, CheckInput{Target: target, Stage: stage, Segments: segments})
	if err != nil {
		return Result{}, nil, err
	}
	redacted, _ := redactResult(segments, result)
	return result, redacted, nil
}
