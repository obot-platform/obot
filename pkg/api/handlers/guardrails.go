package handlers

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/guardrails"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"gorm.io/gorm"
)

// GuardrailsHandler manages guardrail policies and guardrails.
type GuardrailsHandler struct {
	engine *guardrails.Engine
}

// GuardrailResponse is a guardrail as the API returns it: sensitive settings are
// removed from its configuration and only named in ConfiguredSecrets.
type GuardrailResponse struct {
	gatewaytypes.Guardrail
	ConfiguredSecrets []string `json:"configuredSecrets"`
	// Error explains why the guardrail can't be used, such as a missing required setting.
	Error string `json:"error,omitempty"`
}

type guardrailConditionRequest struct {
	Target    string `json:"target"`
	Condition string `json:"condition"`
}

type guardrailTestRequest struct {
	Target string `json:"target"`
	Stage  string `json:"stage"`
	Text   string `json:"text"`
}

type guardrailTestResponse struct {
	Findings     []guardrails.Finding `json:"findings"`
	RedactedText string               `json:"redactedText,omitempty"`
	CanRedact    bool                 `json:"canRedact"`
	Error        string               `json:"error,omitempty"`
}

// guardrailFieldOptionsRequest asks for a field's choices with a guardrail's settings as
// entered in its form. Sensitive settings left empty use the saved guardrail's, when
// GuardrailID names one and its destination hasn't changed.
type guardrailFieldOptionsRequest struct {
	GuardrailID string          `json:"guardrailID,omitempty"`
	Config      json.RawMessage `json:"config"`
}

func NewGuardrailsHandler(engine *guardrails.Engine) *GuardrailsHandler {
	return &GuardrailsHandler{engine: engine}
}

func (h *GuardrailsHandler) invalidate() {
	if h.engine != nil {
		h.engine.Invalidate()
	}
}

// Providers lists the available guardrail types and their settings.
func (h *GuardrailsHandler) Providers(req api.Context) error {
	return req.Write(map[string]any{"items": guardrails.Providers()})
}

// FieldOptions loads a guardrail field's choices from the service its settings point at,
// such as the entity types a Presidio Analyzer supports.
func (h *GuardrailsHandler) FieldOptions(req api.Context) error {
	if h.engine == nil {
		return types.NewErrHTTP(http.StatusServiceUnavailable, "guardrails are not available")
	}
	typ, field := req.PathValue("type"), req.PathValue("field")
	info, ok := guardrails.Provider(typ)
	if !ok {
		return types.NewErrNotFound("unknown guardrail type %q", typ)
	}
	if !slices.ContainsFunc(info.Fields, func(f guardrails.Field) bool { return f.Name == field && f.OptionsFromService }) {
		return types.NewErrNotFound("%s guardrails can't load choices for %q", info.Label, field)
	}
	var in guardrailFieldOptionsRequest
	if err := req.Read(&in); err != nil {
		return types.NewErrBadRequest("invalid request: %v", err)
	}
	candidate := gatewaytypes.Guardrail{Name: info.Label, Type: typ, Config: in.Config}
	current := map[string]string{}
	if in.GuardrailID != "" {
		existing, err := req.GatewayClient.GetGuardrail(req.Context(), in.GuardrailID)
		if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && existing.Type != typ) {
			return types.NewErrNotFound("guardrail %q not found", in.GuardrailID)
		} else if err != nil {
			return err
		}
		if current, err = req.GatewayClient.GuardrailSecrets(req.Context(), existing.ID); err != nil {
			return err
		}
		// Saved credentials are only sent where the saved guardrail sends them.
		if err := checkDestinationChange(existing, &candidate, current); err != nil {
			return err
		}
	}
	secrets, err := splitGuardrailSecrets(&candidate, current)
	if err != nil {
		return err
	}
	if err := checkGuardrailDestinations(req.Context(), candidate); err != nil {
		return err
	}
	config := map[string]any{}
	_ = json.Unmarshal(candidate.Config, &config)
	options, err := h.engine.FieldOptions(req.Context(), typ, field, config, secrets)
	if err != nil {
		return types.NewErrHTTP(http.StatusBadGateway, err.Error())
	}
	if options == nil {
		options = []string{}
	}
	return req.Write(map[string]any{"items": options})
}

func (h *GuardrailsHandler) ListPolicies(req api.Context) error {
	policies, err := req.GatewayClient.ListGuardrailPoliciesForProfile(req.Context(), req.URL.Query().Get("profileID"))
	if err != nil {
		return err
	}
	return req.Write(map[string]any{"items": policies})
}

func (h *GuardrailsHandler) GetPolicy(req api.Context) error {
	policy, err := getGuardrailPolicy(req)
	if err != nil {
		return err
	}
	return req.Write(policy)
}

func (h *GuardrailsHandler) CreatePolicy(req api.Context) error {
	var policy gatewaytypes.GuardrailPolicy
	if err := req.Read(&policy); err != nil {
		return types.NewErrBadRequest("invalid policy: %v", err)
	}
	policy.ID = ""
	if err := validateGuardrailPolicy(req, &policy); err != nil {
		return err
	}
	if err := req.GatewayClient.CreateGuardrailPolicy(req.Context(), &policy); err != nil {
		return err
	}
	h.invalidate()
	if err := confirmProfile(req, policy.ProfileID, func() error {
		defer h.invalidate()
		return req.GatewayClient.DeleteGuardrailPolicy(req.Context(), policy.ID)
	}); err != nil {
		return err
	}
	return req.WriteCreated(policy)
}

func (h *GuardrailsHandler) UpdatePolicy(req api.Context) error {
	existing, err := getGuardrailPolicy(req)
	if err != nil {
		return err
	}
	var policy gatewaytypes.GuardrailPolicy
	if err := req.Read(&policy); err != nil {
		return types.NewErrBadRequest("invalid policy: %v", err)
	}
	policy.ID = existing.ID
	if err := validateGuardrailPolicy(req, &policy); err != nil {
		return err
	}
	if err := req.GatewayClient.UpdateGuardrailPolicy(req.Context(), &policy); err != nil {
		return err
	}
	h.invalidate()
	if err := confirmProfile(req, policy.ProfileID, func() error {
		defer h.invalidate()
		return req.GatewayClient.DeleteGuardrailPolicy(req.Context(), policy.ID)
	}); err != nil {
		return err
	}
	updated, err := getGuardrailPolicy(req)
	if err != nil {
		return err
	}
	return req.Write(updated)
}

func (h *GuardrailsHandler) DeletePolicy(req api.Context) error {
	if _, err := getGuardrailPolicy(req); err != nil {
		return err
	}
	if err := req.GatewayClient.DeleteGuardrailPolicy(req.Context(), req.PathValue("policy_id")); err != nil {
		return err
	}
	h.invalidate()
	req.WriteHeader(http.StatusNoContent)
	return nil
}

// ValidateCondition compiles a policy condition, so the UI can report errors as it's typed.
func (h *GuardrailsHandler) ValidateCondition(req api.Context) error {
	var in guardrailConditionRequest
	if err := req.Read(&in); err != nil {
		return types.NewErrBadRequest("invalid request: %v", err)
	}
	if in.Target != gatewaytypes.GuardrailTargetLLM && in.Target != gatewaytypes.GuardrailTargetMCP {
		return types.NewErrBadRequest("target must be llm or mcp")
	}
	result := map[string]any{"valid": true}
	if strings.TrimSpace(in.Condition) != "" {
		if _, err := guardrails.CompileCondition(in.Target, in.Condition); err != nil {
			result = map[string]any{"valid": false, "error": err.Error()}
		}
	}
	return req.Write(result)
}

func (h *GuardrailsHandler) ListGuardrails(req api.Context) error {
	list, err := req.GatewayClient.ListGuardrails(req.Context())
	if err != nil {
		return err
	}
	items := make([]GuardrailResponse, 0, len(list))
	for _, p := range list {
		resp, err := guardrailResponse(req, p)
		if err != nil {
			return err
		}
		items = append(items, resp)
	}
	return req.Write(map[string]any{"items": items})
}

func (h *GuardrailsHandler) GetGuardrail(req api.Context) error {
	guardrail, err := getGuardrail(req)
	if err != nil {
		return err
	}
	resp, err := guardrailResponse(req, *guardrail)
	if err != nil {
		return err
	}
	return req.Write(resp)
}

func (h *GuardrailsHandler) CreateGuardrail(req api.Context) error {
	var guardrail gatewaytypes.Guardrail
	if err := req.Read(&guardrail); err != nil {
		return types.NewErrBadRequest("invalid guardrail: %v", err)
	}
	guardrail.ID = ""
	secrets, err := splitGuardrailSecrets(&guardrail, nil)
	if err != nil {
		return err
	}
	if err := checkGuardrailDestinations(req.Context(), guardrail); err != nil {
		return err
	}
	if err := req.GatewayClient.CreateGuardrail(req.Context(), &guardrail, secrets); err != nil {
		return err
	}
	h.invalidate()
	resp, err := guardrailResponse(req, guardrail)
	if err != nil {
		return err
	}
	return req.WriteCreated(resp)
}

// UpdateGuardrail saves a guardrail. Sensitive settings that are omitted or empty keep their
// current values.
func (h *GuardrailsHandler) UpdateGuardrail(req api.Context) error {
	existing, err := getGuardrail(req)
	if err != nil {
		return err
	}
	var guardrail gatewaytypes.Guardrail
	if err := req.Read(&guardrail); err != nil {
		return types.NewErrBadRequest("invalid guardrail: %v", err)
	}
	guardrail.ID = existing.ID
	if guardrail.Type != "" && guardrail.Type != existing.Type {
		return types.NewErrBadRequest("a guardrail's type can't be changed")
	}
	guardrail.Type = existing.Type
	current, err := req.GatewayClient.GuardrailSecrets(req.Context(), existing.ID)
	if err != nil {
		return err
	}
	if err := checkDestinationChange(existing, &guardrail, current); err != nil {
		return err
	}
	secrets, err := splitGuardrailSecrets(&guardrail, current)
	if err != nil {
		return err
	}
	if err := checkGuardrailDestinations(req.Context(), guardrail); err != nil {
		return err
	}
	if err := req.GatewayClient.UpdateGuardrail(req.Context(), &guardrail, secrets); err != nil {
		return err
	}
	h.invalidate()
	updated, err := getGuardrail(req)
	if err != nil {
		return err
	}
	resp, err := guardrailResponse(req, *updated)
	if err != nil {
		return err
	}
	return req.Write(resp)
}

func (h *GuardrailsHandler) DeleteGuardrail(req api.Context) error {
	guardrail, err := getGuardrail(req)
	if err != nil {
		return err
	}
	policies, err := req.GatewayClient.ListGuardrailPolicies(req.Context())
	if err != nil {
		return err
	}
	for _, r := range policies {
		if slices.Contains(r.GuardrailIDs, guardrail.ID) {
			return types.NewErrHTTP(http.StatusConflict, fmt.Sprintf("guardrail is used by policy %q; remove it from the policy first", r.Name))
		}
	}
	var filters v1.MCPWebhookValidationList
	if err := req.List(&filters); err != nil {
		return err
	}
	for _, f := range filters.Items {
		if g := f.Spec.Manifest.Guardrail; g != nil && slices.Contains(g.GuardrailIDs, guardrail.ID) {
			return types.NewErrHTTP(http.StatusConflict, fmt.Sprintf("guardrail is used by filter %q; remove it from the filter first", cmp.Or(f.Spec.Manifest.Name, f.Name)))
		}
	}
	if err := req.GatewayClient.DeleteGuardrail(req.Context(), guardrail.ID); err != nil {
		return err
	}
	h.invalidate()
	req.WriteHeader(http.StatusNoContent)
	return nil
}

// TestGuardrail runs a saved guardrail against sample text.
func (h *GuardrailsHandler) TestGuardrail(req api.Context) error {
	if h.engine == nil {
		return types.NewErrHTTP(http.StatusServiceUnavailable, "guardrails are not available")
	}
	guardrail, err := getGuardrail(req)
	if err != nil {
		return err
	}
	var in guardrailTestRequest
	if err := req.Read(&in); err != nil {
		return types.NewErrBadRequest("invalid request: %v", err)
	}
	if in.Target == "" {
		in.Target = gatewaytypes.GuardrailTargetLLM
	}
	if in.Stage == "" {
		in.Stage = guardrails.StageInput
	}
	secrets, err := req.GatewayClient.GuardrailSecrets(req.Context(), guardrail.ID)
	if err != nil {
		return err
	}
	result, redacted, err := h.engine.TestGuardrail(req.Context(), *guardrail, secrets, in.Target, in.Stage, in.Text)
	if err != nil {
		return req.Write(guardrailTestResponse{Findings: []guardrails.Finding{}, Error: err.Error()})
	}
	resp := guardrailTestResponse{Findings: result.Findings, CanRedact: redacted != nil}
	if resp.Findings == nil {
		resp.Findings = []guardrails.Finding{}
	}
	if len(redacted) > 0 && len(result.Findings) > 0 {
		resp.RedactedText = redacted[0].Text
	}
	return req.Write(resp)
}

func getGuardrailPolicy(req api.Context) (*gatewaytypes.GuardrailPolicy, error) {
	policy, err := req.GatewayClient.GetGuardrailPolicy(req.Context(), req.PathValue("policy_id"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, types.NewErrNotFound("guardrail policy %s not found", req.PathValue("policy_id"))
	}
	return policy, err
}

func getGuardrail(req api.Context) (*gatewaytypes.Guardrail, error) {
	guardrail, err := req.GatewayClient.GetGuardrail(req.Context(), req.PathValue("guardrail_id"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, types.NewErrNotFound("guardrail %s not found", req.PathValue("guardrail_id"))
	}
	return guardrail, err
}

func validateGuardrailPolicy(req api.Context, policy *gatewaytypes.GuardrailPolicy) error {
	policy.Name = strings.TrimSpace(policy.Name)
	if policy.Name == "" {
		return types.NewErrBadRequest("name is required")
	}
	switch policy.Target {
	case gatewaytypes.GuardrailTargetLLM:
	case "":
		policy.Target = gatewaytypes.GuardrailTargetLLM
	case gatewaytypes.GuardrailTargetMCP:
		// MCP traffic is checked by guardrail filters, which share the targeting, ordering
		// and auditing of other MCP filters.
		return types.NewErrBadRequest("guardrail policies apply to LLM traffic only; check MCP traffic with a guardrail filter (MCP Servers > Filters)")
	default:
		return types.NewErrBadRequest("target must be llm")
	}
	switch policy.ApplyTo {
	case gatewaytypes.GuardrailApplyInput, gatewaytypes.GuardrailApplyOutput, gatewaytypes.GuardrailApplyBoth:
	case "":
		policy.ApplyTo = gatewaytypes.GuardrailApplyBoth
	default:
		return types.NewErrBadRequest("applyTo must be input, output or both")
	}
	switch policy.Action {
	case gatewaytypes.GuardrailActionBlock, gatewaytypes.GuardrailActionRedact, gatewaytypes.GuardrailActionLog:
	default:
		return types.NewErrBadRequest("action must be block, redact or log")
	}
	switch policy.InputScope {
	case gatewaytypes.GuardrailScopeAll, gatewaytypes.GuardrailScopeLatest:
	case "":
		policy.InputScope = gatewaytypes.GuardrailScopeLatest
	default:
		return types.NewErrBadRequest("inputScope must be all or latest")
	}
	if policy.SamplingRate == 0 {
		policy.SamplingRate = 100
	}
	if policy.SamplingRate < 1 || policy.SamplingRate > 100 {
		return types.NewErrBadRequest("samplingRate must be between 1 and 100")
	}
	if strings.TrimSpace(policy.Condition) != "" {
		if _, err := guardrails.CompileCondition(policy.Target, policy.Condition); err != nil {
			return types.NewErrBadRequest("invalid condition: %v", err)
		}
	}
	if len(policy.GuardrailIDs) == 0 {
		return types.NewErrBadRequest("at least one guardrail is required")
	}
	for _, id := range policy.GuardrailIDs {
		if _, err := req.GatewayClient.GetGuardrail(req.Context(), id); errors.Is(err, gorm.ErrRecordNotFound) {
			return types.NewErrBadRequest("guardrail %s does not exist", id)
		} else if err != nil {
			return err
		}
	}
	return requireProfile(req, policy.ProfileID)
}

// checkDestinationChange refuses an update that changes where a guardrail's credentials are
// sent (an endpoint, or a region in a host name) while keeping the stored credentials.
// Otherwise an admin could point the guardrail at their own server, run a test, and receive
// credentials the API never returns.
func checkDestinationChange(existing *gatewaytypes.Guardrail, updated *gatewaytypes.Guardrail, current map[string]string) error {
	info, ok := guardrails.Provider(existing.Type)
	if !ok || len(current) == 0 {
		return nil
	}
	oldConfig, newConfig := map[string]any{}, map[string]any{}
	_ = json.Unmarshal(existing.Config, &oldConfig)
	_ = json.Unmarshal(updated.Config, &newConfig)
	var changed, kept []string
	for _, f := range info.Fields {
		switch {
		case f.Destination && configString(oldConfig[f.Name]) != configString(newConfig[f.Name]):
			changed = append(changed, f.Label)
		case f.Sensitive && current[f.Name] != "":
			entered, err := enteredSecret(newConfig, f)
			if err != nil {
				return err
			}
			if entered == "" {
				kept = append(kept, f.Label)
			}
		}
	}
	if len(changed) == 0 || len(kept) == 0 {
		return nil
	}
	return types.NewErrBadRequest("changing %s requires re-entering %s", strings.Join(changed, " and "), strings.Join(kept, " and "))
}

// enteredSecret returns the value given for a sensitive setting, or "" when it was omitted,
// which keeps the saved value. Checking whether a stored credential is kept and keeping it
// must agree, so both use this: a value that isn't a string is refused rather than read as
// omitted.
func enteredSecret(config map[string]any, f guardrails.Field) (string, error) {
	v, ok := config[f.Name]
	if !ok || v == nil {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", types.NewErrBadRequest("%s must be a string", f.Label)
	}
	return strings.TrimSpace(s), nil
}

func configString(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

// checkGuardrailDestinations refuses a guardrail that would send content or credentials to an
// address guardrails may not use.
func checkGuardrailDestinations(ctx context.Context, guardrail gatewaytypes.Guardrail) error {
	config := map[string]any{}
	_ = json.Unmarshal(guardrail.Config, &config)
	if err := guardrails.CheckDestinations(ctx, guardrail.Type, config); err != nil {
		return types.NewErrBadRequest("%v", err)
	}
	return nil
}

// splitGuardrailSecrets validates a guardrail and moves its sensitive settings out of its
// configuration. current holds the saved secrets, which omitted sensitive settings keep.
func splitGuardrailSecrets(guardrail *gatewaytypes.Guardrail, current map[string]string) (map[string]string, error) {
	guardrail.Name = strings.TrimSpace(guardrail.Name)
	if guardrail.Name == "" {
		return nil, types.NewErrBadRequest("name is required")
	}
	info, ok := guardrails.Provider(guardrail.Type)
	if !ok {
		return nil, types.NewErrBadRequest("unknown guardrail type %q", guardrail.Type)
	}
	config := map[string]any{}
	if len(guardrail.Config) > 0 && string(guardrail.Config) != "null" {
		if err := json.Unmarshal(guardrail.Config, &config); err != nil {
			return nil, types.NewErrBadRequest("config must be an object: %v", err)
		}
	}
	secrets := map[string]string{}
	for _, f := range info.Fields {
		if !f.Sensitive {
			continue
		}
		value, err := enteredSecret(config, f)
		if err != nil {
			return nil, err
		}
		delete(config, f.Name)
		if value == "" {
			value = current[f.Name]
		}
		if value != "" {
			secrets[f.Name] = value
		}
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	guardrail.Config = raw
	return secrets, nil
}

func guardrailResponse(req api.Context, guardrail gatewaytypes.Guardrail) (GuardrailResponse, error) {
	resp := GuardrailResponse{Guardrail: guardrail, ConfiguredSecrets: []string{}}
	secrets, err := req.GatewayClient.GuardrailSecrets(req.Context(), guardrail.ID)
	if err != nil {
		return resp, err
	}
	resp.Error = guardrails.GuardrailError(guardrail, secrets)
	info, ok := guardrails.Provider(guardrail.Type)
	if !ok {
		return resp, nil
	}
	config := map[string]any{}
	_ = json.Unmarshal(guardrail.Config, &config)
	for _, f := range info.Fields {
		if f.Sensitive {
			delete(config, f.Name)
			if secrets[f.Name] != "" {
				resp.ConfiguredSecrets = append(resp.ConfiguredSecrets, f.Name)
			}
		}
	}
	resp.Config, _ = json.Marshal(config)
	return resp, nil
}
