package handlers

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	types "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/server/requestinfo"
	"github.com/obot-platform/obot/pkg/enforcement"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/utils"
	"gorm.io/gorm"
)

type EnforcementHandler struct {
	// serverURL is Obot's own base URL, parsed once so that classifying a
	// device-reported call as Obot-hosted never has to handle a parse error.
	// It is nil when no server URL is configured.
	serverURL *url.URL
}

func NewEnforcementHandler(serverURL string) (*EnforcementHandler, error) {
	if serverURL == "" {
		return &EnforcementHandler{}, nil
	}
	parsed, err := url.Parse(serverURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse server URL %q: %w", serverURL, err)
	}
	return &EnforcementHandler{serverURL: parsed}, nil
}

// Decide handles POST /api/enforcement/decisions.
func (h *EnforcementHandler) Decide(req api.Context) error {
	var in types.EnforcementDecisionRequest
	if err := req.Read(&in); err != nil {
		return types.NewErrBadRequest("failed to read input: %v", err)
	}

	extra := req.User.GetExtra()
	deviceID := utils.FirstSet(extra["device_id"]...)

	// Resolve the fleet configuration strictly from the authenticated identity.
	// A caller that is not an enrolled device has no fleet to enforce against:
	// deny without recording, so only devices can write decision rows.
	configID, ok := parseConfigurationID(utils.FirstSet(extra["mdm_configuration_id"]...))
	if deviceID == "" || !ok {
		return respondDecision(req, enforcement.Decision{
			Allow:  false,
			Reason: "no MDM configuration is associated with this device",
		})
	}

	obotHosted := h.isObotHosted(in.Server.URL)
	base := gtypes.EnforcementDecisionLog{
		MDMConfigurationID: configID,
		DeviceID:           deviceID,
		ClientIP:           requestinfo.GetSourceIP(req.Request),
		ObotHosted:         obotHosted,
	}

	policy, err := req.GatewayClient.GetMDMConfigurationEnforcement(req.Context(), configID)
	if err != nil {
		return h.recordAndRespond(req, base, in, enforcement.Decision{
			Allow:  false,
			Reason: "MDM configuration could not be loaded",
		})
	}

	// Enforcement is opt-in per fleet. When it is disabled there is nothing to
	// enforce: allow the call unconditionally and skip logging.
	if !policy.Enabled {
		return respondDecision(req, enforcement.Decision{
			Allow:  true,
			Reason: "enforcement is not enabled",
		})
	}

	decision := enforcement.Evaluate(normalizedCallFromRequest(in, obotHosted), policy.Allowlist)
	return h.recordAndRespond(req, base, in, decision)
}

func verdict(decision enforcement.Decision) string {
	if decision.Allow {
		return types.EnforcementDecisionAllow
	}
	return types.EnforcementDecisionDeny
}

// respondDecision returns the synchronous verdict to the device without
// recording it, for the paths that deliberately produce no audit row.
func respondDecision(req api.Context, decision enforcement.Decision) error {
	return req.Write(types.EnforcementDecisionResponse{
		Decision: verdict(decision),
		Reason:   decision.Reason,
	})
}

// recordAndRespond stamps the normalized call onto the decision-log row, records
// it (buffered/async), and returns the synchronous verdict to the device.
//
// The device-supplied strings are bounded here, on their way to storage only. The
// verdict is decided from the values as they arrived (normalizedCallFromRequest):
// truncating before evaluation would be a bypass, because a long URL cut mid-path
// can land exactly on an allowlisted path prefix that the real URL only extended.
func (h *EnforcementHandler) recordAndRespond(req api.Context, entry gtypes.EnforcementDecisionLog, in types.EnforcementDecisionRequest, decision enforcement.Decision) error {
	entry.CreatedAt = time.Now().UTC()
	entry.Agent = truncateRunes(in.Agent, maxIdentifierRunes)
	entry.Tool = truncateRunes(in.Tool, maxIdentifierRunes)
	entry.Kind = truncateRunes(in.Kind, maxIdentifierRunes)
	entry.ServerName = truncateRunes(in.ServerName, maxIdentifierRunes)
	entry.Decision = verdict(decision)
	entry.Reason = decision.Reason
	entry.ServerURL = truncateRunes(sanitizeServerURL(in.Server.URL), maxServerURLRunes)
	entry.ServerHostname = truncateRunes(serverHostname(in.Server), maxIdentifierRunes)
	entry.ServerCommand = truncateRunes(sanitizeServerCommand(in.Server.Command), maxServerURLRunes)
	entry.ServerConnector = truncateRunes(in.Server.Connector, maxIdentifierRunes)
	entry.Unresolved = in.Unresolved
	if in.Unresolved {
		entry.UnresolvedReason = sanitizeUnresolvedReason(in.UnresolvedReason)
	}
	if in.Server.Package != nil {
		entry.ServerPackageSource = truncateRunes(string(in.Server.Package.Source), maxIdentifierRunes)
		entry.ServerPackageName = truncateRunes(in.Server.Package.Name, maxIdentifierRunes)
		entry.ServerPackageVersion = truncateRunes(in.Server.Package.Version, maxIdentifierRunes)
	}

	req.GatewayClient.LogEnforcementDecision(entry)

	return respondDecision(req, decision)
}

// ListDecisions handles GET /api/enforcement-decisions (admin-only).
func (h *EnforcementHandler) ListDecisions(req api.Context) error {
	opts, err := parseEnforcementDecisionOptions(req.URL.Query())
	if err != nil {
		return err
	}
	if err := opts.Validate(); err != nil {
		return types.NewErrBadRequest("%v", err)
	}
	if opts.Limit == 0 {
		opts.Limit = 100
	}

	logs, total, err := req.GatewayClient.GetEnforcementDecisions(req.Context(), opts)
	if err != nil {
		return err
	}

	items := make([]types.EnforcementDecisionEvent, 0, len(logs))
	for i := range logs {
		items = append(items, presentEnforcementDecision(logs[i]))
	}

	return req.Write(types.EnforcementDecisionEventResponse{
		Items:  items,
		Total:  total,
		Limit:  opts.Limit,
		Offset: opts.Offset,
	})
}

// parseEnforcementDecisionID reads a decision id from the request path.
func parseEnforcementDecisionID(req api.Context) (uint, error) {
	id := req.PathValue("id")
	if id == "" {
		return 0, types.NewErrBadRequest("missing enforcement decision id")
	}
	parsed, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return 0, types.NewErrBadRequest("invalid enforcement decision id: %v", err)
	}
	return uint(parsed), nil
}

// GetDecision handles GET /api/enforcement-decisions/{id} (admin-only).
func (h *EnforcementHandler) GetDecision(req api.Context) error {
	id, err := parseEnforcementDecisionID(req)
	if err != nil {
		return err
	}

	decision, err := req.GatewayClient.GetEnforcementDecision(req.Context(), id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return types.NewErrNotFound("enforcement decision %d not found", id)
	} else if err != nil {
		return err
	}

	return req.Write(presentEnforcementDecision(*decision))
}

// CheckDecisionAllowlist handles GET /api/enforcement-decisions/allowlist-check/{id}
// (admin-only). It replays a recorded decision against its fleet's current
// allowlist. It records nothing in the decision log.
func (h *EnforcementHandler) CheckDecisionAllowlist(req api.Context) error {
	id, err := parseEnforcementDecisionID(req)
	if err != nil {
		return err
	}

	log, err := req.GatewayClient.GetEnforcementDecision(req.Context(), id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return types.NewErrNotFound("enforcement decision %d not found", id)
	} else if err != nil {
		return err
	}

	policy, err := req.GatewayClient.GetMDMConfigurationEnforcement(req.Context(), log.MDMConfigurationID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return types.NewErrNotFound("MDM configuration %d not found", log.MDMConfigurationID)
	} else if err != nil {
		return err
	}

	decision := enforcement.Evaluate(
		normalizedCallFromDecisionLog(*log, h.isObotHosted(log.ServerURL)),
		policy.Allowlist,
	)

	return req.Write(types.EnforcementDecisionAllowlistCheck{
		ID:                 strconv.FormatUint(uint64(log.ID), 10),
		AllowlistDecision:  verdict(decision),
		AllowlistReason:    decision.Reason,
		EnforcementEnabled: policy.Enabled,
	})
}

// enforcementDecisionFilters are the filter keys the decision-log UI may request
// options for. "decision" is a fixed enum served independently of the data.
var enforcementDecisionFilters = map[string]struct{}{
	"agent":    {},
	"tool":     {},
	"kind":     {},
	"server":   {},
	"decision": {},
	"actor":    {},
}

// ListFilterOptions handles GET /api/enforcement-decisions/filter-options/{filter} (admin-only).
func (h *EnforcementHandler) ListFilterOptions(req api.Context) error {
	filter := req.PathValue("filter")
	if filter == "" {
		return types.NewErrBadRequest("missing filter")
	}
	if _, ok := enforcementDecisionFilters[filter]; !ok {
		return types.NewErrBadRequest("invalid filter: %s", filter)
	}

	if filter == "decision" {
		return req.Write(map[string]any{
			"options": []string{types.EnforcementDecisionAllow, types.EnforcementDecisionDeny},
		})
	}

	opts, err := parseEnforcementDecisionOptions(req.URL.Query())
	if err != nil {
		return err
	}

	options, err := req.GatewayClient.GetEnforcementDecisionFilterOptions(req.Context(), filter, opts)
	if err != nil {
		return err
	}
	sort.Strings(options)
	return req.Write(map[string]any{"options": options})
}

func parseEnforcementDecisionOptions(query url.Values) (gateway.EnforcementDecisionOptions, error) {
	configurationIDs, err := parseMultiValueUint(query, "mdm_configuration_id")
	if err != nil {
		return gateway.EnforcementDecisionOptions{}, err
	}

	opts := gateway.EnforcementDecisionOptions{
		MDMConfigurationID: configurationIDs,
		Actor:              parseMultiValue(query, "actor"),
		Agent:              parseMultiValue(query, "agent"),
		Server:             parseMultiValue(query, "server"),
		Tool:               parseMultiValue(query, "tool"),
		Kind:               parseMultiValue(query, "kind"),
		Decision:           parseMultiValue(query, "decision"),
		SortBy:             query.Get("sort_by"),
		SortOrder:          query.Get("sort_order"),
		Query:              strings.TrimSpace(query.Get("query")),
	}

	if startTime := query.Get("start_time"); startTime != "" {
		if t, err := time.Parse(time.RFC3339, startTime); err == nil {
			opts.StartTime = t
		}
	}
	if endTime := query.Get("end_time"); endTime != "" {
		if t, err := time.Parse(time.RFC3339, endTime); err == nil {
			opts.EndTime = t
		}
	}
	if limitStr := query.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			opts.Limit = l
		}
	}
	if offsetStr := query.Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			opts.Offset = o
		}
	}
	return opts, nil
}

// presentEnforcementDecision converts a stored row to the public decision event.
// The resolved server identity is clear-text and always populated (list and
// detail views alike).
func presentEnforcementDecision(log gtypes.EnforcementDecisionLog) types.EnforcementDecisionEvent {
	event := types.EnforcementDecisionEvent{
		ID:                 strconv.FormatUint(uint64(log.ID), 10),
		CreatedAt:          *types.NewTime(log.CreatedAt),
		MDMConfigurationID: log.MDMConfigurationID,
		DeviceID:           log.DeviceID,
		ClientIP:           log.ClientIP,
		Agent:              log.Agent,
		Tool:               log.Tool,
		Kind:               log.Kind,
		ServerName:         log.ServerName,
		ObotHosted:         log.ObotHosted,
		Decision:           log.Decision,
		Reason:             log.Reason,
		Unresolved:         log.Unresolved,
		UnresolvedReason:   log.UnresolvedReason,
	}
	server := types.EnforcementDecisionServer{
		URL:       log.ServerURL,
		Command:   log.ServerCommand,
		Hostname:  log.ServerHostname,
		Connector: log.ServerConnector,
	}
	if log.ServerPackageName != "" || log.ServerPackageSource != "" {
		server.Package = &types.AllowlistServerPackage{
			Source:  types.AllowlistServerPackageSource(log.ServerPackageSource),
			Name:    log.ServerPackageName,
			Version: log.ServerPackageVersion,
		}
	}
	event.Server = &server
	return event
}

func normalizedCallFromRequest(in types.EnforcementDecisionRequest, obotHosted bool) enforcement.NormalizedCall {
	call := enforcement.NormalizedCall{
		Agent:      in.Agent,
		Tool:       in.Tool,
		Kind:       in.Kind,
		ServerName: in.ServerName,
		ObotHosted: obotHosted,
		Server: enforcement.ServerIdentity{
			URL:     in.Server.URL,
			Command: in.Server.Command,
			// Hostname is left empty on purpose so the evaluator derives it from the
			// URL. Honoring the device-reported value would let a report name a
			// hostname its own URL contradicts, and a hostname allowlist entry would
			// then match a call to somewhere else entirely. See serverHostname.
			Connector: in.Server.Connector,
		},
		Unresolved: in.Unresolved,
	}
	if in.Unresolved {
		call.UnresolvedReason = sanitizeUnresolvedReason(in.UnresolvedReason)
	}
	if in.Server.Package != nil {
		call.Server.Package = &enforcement.PackageIdentity{
			Source:  in.Server.Package.Source,
			Name:    in.Server.Package.Name,
			Version: in.Server.Package.Version,
		}
	}
	return call
}

func normalizedCallFromDecisionLog(log gtypes.EnforcementDecisionLog, obotHosted bool) enforcement.NormalizedCall {
	call := enforcement.NormalizedCall{
		Agent:      log.Agent,
		Tool:       log.Tool,
		Kind:       log.Kind,
		ServerName: log.ServerName,
		ObotHosted: obotHosted,
		Server: enforcement.ServerIdentity{
			URL:       log.ServerURL,
			Command:   log.ServerCommand,
			Hostname:  log.ServerHostname,
			Connector: log.ServerConnector,
		},
		Unresolved:       log.Unresolved,
		UnresolvedReason: log.UnresolvedReason,
	}
	if log.ServerPackageName != "" || log.ServerPackageSource != "" {
		call.Server.Package = &enforcement.PackageIdentity{
			Source:  types.AllowlistServerPackageSource(log.ServerPackageSource),
			Name:    log.ServerPackageName,
			Version: log.ServerPackageVersion,
		}
	}
	return call
}

// isObotHosted reports whether callURL targets an Obot-hosted MCP server.
func (h *EnforcementHandler) isObotHosted(callURL string) bool {
	if callURL == "" || h.serverURL == nil {
		return false
	}
	call, err := url.Parse(callURL)
	if err != nil {
		return false
	}
	server := h.serverURL
	callHost := call.Hostname()
	serverHost := server.Hostname()
	if callHost == "" || serverHost == "" {
		return false
	}
	if !strings.EqualFold(callHost, serverHost) {
		return false
	}
	if !strings.EqualFold(call.Scheme, server.Scheme) {
		return false
	}
	return enforcement.NormalizedPort(call) == enforcement.NormalizedPort(server)
}

// sanitizeServerURL keeps only the parts of a device-reported URL that the
// evaluator matches on — scheme, host, port, and path — dropping userinfo, the
// query string, and the fragment.
func sanitizeServerURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		// An unparseable URL matches no allowlist entry. Keep whatever precedes
		// a query or fragment for diagnosis and drop the rest.
		if i := strings.IndexAny(raw, "?#"); i >= 0 {
			return raw[:i]
		}
		return raw
	}
	u.User = nil
	u.RawQuery, u.ForceQuery = "", false
	u.Fragment, u.RawFragment = "", ""
	return u.String()
}

// sanitizeServerCommand keeps only the executable of a device-reported stdio
// command and drops its arguments. The evaluator never matches on the command at
// all — a package server is identified by source/name/version — while arguments
// routinely carry API keys and inline environment assignments. The executable is
// kept anyway because for a server with no package or URL identity it is the only
// thing in the row that says what ran. A quoted path containing spaces is cut at
// the first space: cosmetic loss, never a secret.
func sanitizeServerCommand(raw string) string {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

const (
	maxUnresolvedReasonRunes = 512
	maxIdentifierRunes       = 256
	maxServerURLRunes        = 2048
)

func sanitizeUnresolvedReason(raw string) string {
	return truncateRunes(strings.TrimSpace(raw), maxUnresolvedReasonRunes)
}

// truncateRunes cuts s to at most maximum runes, never splitting one. It walks
// byte offsets and stops at the cut instead of materializing a []rune, so an
// oversized device-supplied string costs no allocation beyond the kept prefix.
func truncateRunes(s string, maximum int) string {
	count := 0
	for i := range s {
		if count == maximum {
			return s[:i]
		}
		count++
	}
	return s
}

// serverHostname returns the hostname the row should record, always derived from
// the resolved URL.
func serverHostname(server types.EnforcementDecisionServer) string {
	if server.URL == "" {
		return ""
	}
	u, err := url.Parse(server.URL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func parseMultiValueUint(query url.Values, key string) ([]uint, error) {
	values := parseMultiValue(query, key)
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]uint, 0, len(values))
	for _, value := range values {
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return nil, types.NewErrBadRequest("invalid %s %q: must be a non-negative integer", key, value)
		}
		out = append(out, uint(parsed))
	}
	return out, nil
}

func parseConfigurationID(raw string) (uint, bool) {
	if raw == "" {
		return 0, false
	}
	parsed, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return uint(parsed), true
}
