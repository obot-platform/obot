package handlers

import (
	"errors"
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
	"gorm.io/gorm"
)

type EnforcementHandler struct {
	serverURL string
}

func NewEnforcementHandler(serverURL string) *EnforcementHandler {
	return &EnforcementHandler{serverURL: serverURL}
}

// Decide handles POST /api/enforcement/decisions.
func (h *EnforcementHandler) Decide(req api.Context) error {
	var in types.EnforcementDecisionRequest
	if err := req.Read(&in); err != nil {
		return types.NewErrBadRequest("failed to read input: %v", err)
	}

	extra := req.User.GetExtra()
	deviceID := firstExtraValue(extra, "device_id")
	clientIP := requestinfo.GetSourceIP(req.Request)
	obotHosted := h.isObotHosted(in.Server.URL)

	call := normalizedCallFromRequest(in, obotHosted)

	// Resolve the fleet configuration strictly from the authenticated identity.
	configID, ok := parseConfigurationID(firstExtraValue(extra, "mdm_configuration_id"))
	if !ok {
		return h.recordAndRespond(req, gtypes.EnforcementDecisionLog{
			MDMConfigurationID: configID,
			DeviceID:           deviceID,
			ClientIP:           clientIP,
			ObotHosted:         obotHosted,
		}, in, enforcement.Decision{
			Allow:  false,
			Reason: "no MDM configuration is associated with this device",
		})
	}

	base := gtypes.EnforcementDecisionLog{
		MDMConfigurationID: configID,
		DeviceID:           deviceID,
		ClientIP:           clientIP,
		ObotHosted:         obotHosted,
	}

	config, err := req.GatewayClient.GetMDMConfiguration(req.Context(), configID)
	if err != nil {
		return h.recordAndRespond(req, base, in, enforcement.Decision{
			Allow:  false,
			Reason: "MDM configuration could not be loaded",
		})
	}

	// Enforcement is opt-in per fleet. When it is disabled there is nothing to
	// enforce: allow the call unconditionally and skip logging.
	if !config.EnforcementEnabled {
		return req.Write(types.EnforcementDecisionResponse{
			Decision: types.EnforcementDecisionAllow,
			Reason:   "enforcement is not enabled",
		})
	}

	decision := enforcement.Evaluate(call, config.EnforcementAllowlist)
	return h.recordAndRespond(req, base, in, decision)
}

// recordAndRespond stamps the normalized call onto the decision-log row, records
// it (buffered/async), and returns the synchronous verdict to the device.
func (h *EnforcementHandler) recordAndRespond(req api.Context, entry gtypes.EnforcementDecisionLog, in types.EnforcementDecisionRequest, decision enforcement.Decision) error {
	verdict := types.EnforcementDecisionDeny
	if decision.Allow {
		verdict = types.EnforcementDecisionAllow
	}

	entry.CreatedAt = time.Now().UTC()
	entry.Agent = in.Agent
	entry.Tool = in.Tool
	entry.Kind = in.Kind
	entry.ServerName = in.ServerName
	entry.Decision = verdict
	entry.Reason = decision.Reason
	entry.ServerURL = in.Server.URL
	entry.ServerHostname = serverHostname(in.Server)
	entry.ServerCommand = in.Server.Command
	if in.Server.Package != nil {
		entry.ServerPackageSource = string(in.Server.Package.Source)
		entry.ServerPackageName = in.Server.Package.Name
		entry.ServerPackageVersion = in.Server.Package.Version
	}

	req.GatewayClient.LogEnforcementDecision(entry)

	return req.Write(types.EnforcementDecisionResponse{
		Decision: verdict,
		Reason:   decision.Reason,
	})
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
		EnforcementDecisionEventList: types.EnforcementDecisionEventList{Items: items},
		Total:                        total,
		Limit:                        opts.Limit,
		Offset:                       opts.Offset,
	})
}

// GetDecision handles GET /api/enforcement-decisions/{id} (admin-only).
func (h *EnforcementHandler) GetDecision(req api.Context) error {
	id := req.PathValue("id")
	if id == "" {
		return types.NewErrBadRequest("missing enforcement decision id")
	}
	parsed, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return types.NewErrBadRequest("invalid enforcement decision id: %v", err)
	}

	decision, err := req.GatewayClient.GetEnforcementDecision(req.Context(), uint(parsed))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return types.NewErrNotFound("enforcement decision %s not found", id)
	} else if err != nil {
		return err
	}

	return req.Write(presentEnforcementDecision(*decision))
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
	opts := gateway.EnforcementDecisionOptions{
		MDMConfigurationID: parseMultiValue(query, "mdm_configuration_id"),
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
	}
	server := types.EnforcementDecisionServer{
		URL:      log.ServerURL,
		Command:  log.ServerCommand,
		Hostname: log.ServerHostname,
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
			URL:      in.Server.URL,
			Command:  in.Server.Command,
			Hostname: in.Server.Hostname,
		},
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

// isObotHosted reports whether callURL targets an Obot-hosted MCP server.
func (h *EnforcementHandler) isObotHosted(callURL string) bool {
	if callURL == "" || h.serverURL == "" {
		return false
	}
	call, err := url.Parse(callURL)
	if err != nil {
		return false
	}
	server, err := url.Parse(h.serverURL)
	if err != nil {
		return false // should never happen
	}
	callHost := call.Hostname()
	serverHost := server.Hostname()
	if callHost == "" || serverHost == "" {
		return false
	}
	return strings.EqualFold(callHost, serverHost)
}

// serverHostname returns the hostname the row should record: the explicit one if
// supplied, otherwise the host derived from the URL.
func serverHostname(server types.EnforcementDecisionServer) string {
	if server.Hostname != "" {
		return server.Hostname
	}
	if server.URL == "" {
		return ""
	}
	u, err := url.Parse(server.URL)
	if err != nil {
		return ""
	}
	return u.Hostname()
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

func firstExtraValue(extra map[string][]string, key string) string {
	values := extra[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
