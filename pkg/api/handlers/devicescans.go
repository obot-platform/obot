package handlers

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	types "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

// dashboardWindowDefault is the default rolling window applied to GetScanStats.
const dashboardWindowDefault = 60 * 24 * time.Hour

// DeviceScansHandler serves the `obot scan` ingest + read API
type DeviceScansHandler struct{}

func NewDeviceScansHandler() *DeviceScansHandler {
	return &DeviceScansHandler{}
}

// Submit handles POST /api/devices/scans. The caller's identity is
// recorded as SubmittedBy; ID and ReceivedAt are server-assigned.
func (*DeviceScansHandler) Submit(req api.Context) error {
	var manifest types.DeviceScanManifest
	if err := req.Read(&manifest); err != nil {
		return err
	}

	scan := gtypes.DeviceScanFromManifest(manifest)
	scan.SubmittedBy = req.User.GetUID()

	if err := req.GatewayClient.InsertDeviceScan(req.Context(), &scan); err != nil {
		return err
	}

	return req.WriteCreated(gtypes.ConvertDeviceScan(scan))
}

// List handles GET /api/devices/scans. Optional submitted_by / device_id
// filters narrow the result.
func (*DeviceScansHandler) List(req api.Context) error {
	opts := parseDeviceScanListOpts(req.URL.Query())
	if opts.Limit == 0 {
		opts.Limit = 100
	}

	scans, total, err := req.GatewayClient.ListDeviceScans(req.Context(), opts)
	if err != nil {
		return err
	}

	items := make([]types.DeviceScan, 0, len(scans))
	for _, s := range scans {
		items = append(items, gtypes.ConvertDeviceScan(s))
	}
	return req.Write(types.DeviceScanResponse{
		DeviceScanList: types.DeviceScanList{Items: items},
		Total:          total,
		Limit:          opts.Limit,
		Offset:         opts.Offset,
	})
}

// Get handles GET /api/devices/scans/{scan_id}. Returns the scan
// envelope plus all child rows (MCP servers, skills, plugins, files).
func (*DeviceScansHandler) Get(req api.Context) error {
	id, err := parseDeviceScanID(req.PathValue("scan_id"))
	if err != nil {
		return err
	}
	scan, err := req.GatewayClient.GetDeviceScan(req.Context(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.NewErrNotFound("device scan %d not found", id)
		}
		return err
	}
	return req.Write(gtypes.ConvertDeviceScan(*scan))
}

// Delete handles DELETE /api/devices/scans/{scan_id}. Idempotent:
// succeeds whether or not a scan with that id existed.
func (*DeviceScansHandler) Delete(req api.Context) error {
	id, err := parseDeviceScanID(req.PathValue("scan_id"))
	if err != nil {
		return err
	}
	return req.GatewayClient.DeleteDeviceScan(req.Context(), id)
}

func parseDeviceScanID(raw string) (uint, error) {
	if raw == "" {
		return 0, types.NewErrBadRequest("missing device scan id")
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, types.NewErrBadRequest("invalid device scan id: %v", err)
	}
	return uint(id), nil
}

func parseDeviceScanListOpts(query url.Values) gateway.DeviceScanListOptions {
	opts := gateway.DeviceScanListOptions{
		SubmittedBy:   parseMultiValueDeviceScan(query, "submitted_by"),
		DeviceID:      parseMultiValueDeviceScan(query, "device_id"),
		GroupByDevice: true,
	}
	if v := query.Get("group_by_device"); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			opts.GroupByDevice = parsed
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
	return opts
}

// parseMultiValueDeviceScan accepts both repeated query params
// (?submitted_by=a&submitted_by=b) and comma-separated values
// (?submitted_by=a,b). Whitespace + empty entries are dropped.
func parseMultiValueDeviceScan(query url.Values, key string) []string {
	values := query[key]
	if len(values) == 0 {
		return nil
	}
	var out []string
	for _, v := range values {
		for part := range strings.SplitSeq(v, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// GetMCPServerDetail handles GET /api/devices/mcp-servers/{config_hash}.
// Returns the all-time aggregate for that hash plus Args, EnvKeys,
// and HeaderKeys.
func (*DeviceScansHandler) GetMCPServerDetail(req api.Context) error {
	hash := req.PathValue("config_hash")
	if hash == "" {
		return types.NewErrBadRequest("missing config_hash")
	}
	detail, err := req.GatewayClient.GetMCPServerDetail(req.Context(), hash)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.NewErrNotFound("config %s not found", hash)
		}
		return err
	}
	return req.Write(convertMCPServerDetail(*detail))
}

// ListMCPServerOccurrences handles
// GET /api/devices/mcp-servers/{config_hash}/occurrences. Returns one
// row per (device, observation) of the given hash from each device's
// all-time latest scan.
func (*DeviceScansHandler) ListMCPServerOccurrences(req api.Context) error {
	hash := req.PathValue("config_hash")
	if hash == "" {
		return types.NewErrBadRequest("missing config_hash")
	}
	q := req.URL.Query()
	limit := 50
	if v := q.Get("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil && l > 0 {
			limit = l
		}
	}
	offset := 0
	if v := q.Get("offset"); v != "" {
		if o, err := strconv.Atoi(v); err == nil && o >= 0 {
			offset = o
		}
	}
	rows, total, err := req.GatewayClient.ListMCPServerOccurrences(req.Context(), hash, limit, offset)
	if err != nil {
		return err
	}
	items := make([]types.DeviceMCPServerOccurrence, 0, len(rows))
	for _, r := range rows {
		items = append(items, types.DeviceMCPServerOccurrence{
			DeviceScanID: r.DeviceScanID,
			DeviceID:     r.DeviceID,
			Client:       r.Client,
			Scope:        r.Scope,
			ScannedAt:    *types.NewTime(r.ScannedAt),
			ID:           r.ID,
		})
	}
	return req.Write(types.DeviceMCPServerOccurrenceResponse{
		DeviceMCPServerOccurrenceList: types.DeviceMCPServerOccurrenceList{Items: items},
		Total:                         total,
		Limit:                         limit,
		Offset:                        offset,
	})
}

// ListSkills handles GET /api/devices/skills. Paginated, sortable,
// optional name LIKE filter and time-window scoping.
func (*DeviceScansHandler) ListSkills(req api.Context) error {
	opts, err := parseSkillStatListOpts(req.URL.Query())
	if err != nil {
		return err
	}
	if opts.Limit == 0 {
		opts.Limit = 50
	}

	rows, total, err := req.GatewayClient.ListSkillStats(req.Context(), opts)
	if err != nil {
		return err
	}
	items := make([]types.DeviceSkillStat, 0, len(rows))
	for _, r := range rows {
		items = append(items, types.DeviceSkillStat{
			Name:             r.Name,
			DeviceCount:      r.DeviceCount,
			UserCount:        r.UserCount,
			ObservationCount: r.ObservationCount,
		})
	}
	return req.Write(types.DeviceSkillStatResponse{
		DeviceSkillStatList: types.DeviceSkillStatList{Items: items},
		Total:               total,
		Limit:               opts.Limit,
		Offset:              opts.Offset,
	})
}

func parseSkillStatListOpts(query url.Values) (gateway.SkillStatListOptions, error) {
	opts := gateway.SkillStatListOptions{
		Name:      strings.TrimSpace(query.Get("name")),
		SortBy:    query.Get("sort_by"),
		SortOrder: query.Get("sort_order"),
	}
	if v := query.Get("start"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return opts, types.NewErrBadRequest("invalid start: %v", err)
		}
		opts.StartTime = t
	}
	if v := query.Get("end"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return opts, types.NewErrBadRequest("invalid end: %v", err)
		}
		opts.EndTime = t
	}
	if v := query.Get("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil && l > 0 {
			opts.Limit = l
		}
	}
	if v := query.Get("offset"); v != "" {
		if o, err := strconv.Atoi(v); err == nil && o >= 0 {
			opts.Offset = o
		}
	}
	return opts, nil
}

// GetSkill handles GET /api/devices/skills/{name}. Returns the
// all-time per-skill aggregate plus representative Description /
// HasScripts / GitRemoteURL / Files from one canonical row.
func (*DeviceScansHandler) GetSkill(req api.Context) error {
	name := req.PathValue("name")
	if name == "" {
		return types.NewErrBadRequest("missing skill name")
	}
	detail, err := req.GatewayClient.GetSkillDetail(req.Context(), name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.NewErrNotFound("skill %q not found", name)
		}
		return err
	}
	return req.Write(types.DeviceSkillDetail{
		DeviceSkillStat: types.DeviceSkillStat{
			Name:             detail.Name,
			DeviceCount:      detail.DeviceCount,
			UserCount:        detail.UserCount,
			ObservationCount: detail.ObservationCount,
		},
		Description:  detail.Description,
		HasScripts:   detail.HasScripts,
		GitRemoteURL: detail.GitRemoteURL,
		Files:        detail.Files,
	})
}

// ListSkillOccurrences handles
// GET /api/devices/skills/{name}/occurrences. Returns one row per
// (device, observation) of the given skill name from each device's
// all-time latest scan.
func (*DeviceScansHandler) ListSkillOccurrences(req api.Context) error {
	name := req.PathValue("name")
	if name == "" {
		return types.NewErrBadRequest("missing skill name")
	}
	q := req.URL.Query()
	limit := 50
	if v := q.Get("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil && l > 0 {
			limit = l
		}
	}
	offset := 0
	if v := q.Get("offset"); v != "" {
		if o, err := strconv.Atoi(v); err == nil && o >= 0 {
			offset = o
		}
	}
	rows, total, err := req.GatewayClient.ListSkillOccurrences(req.Context(), name, limit, offset)
	if err != nil {
		return err
	}
	items := make([]types.DeviceSkillOccurrence, 0, len(rows))
	for _, r := range rows {
		items = append(items, types.DeviceSkillOccurrence{
			DeviceScanID: r.DeviceScanID,
			DeviceID:     r.DeviceID,
			Client:       r.Client,
			Scope:        r.Scope,
			ProjectPath:  r.ProjectPath,
			ScannedAt:    *types.NewTime(r.ScannedAt),
			ID:           r.ID,
		})
	}
	return req.Write(types.DeviceSkillOccurrenceResponse{
		DeviceSkillOccurrenceList: types.DeviceSkillOccurrenceList{Items: items},
		Total:                     total,
		Limit:                     limit,
		Offset:                    offset,
	})
}

// GetScanStats handles GET /api/devices/scan-stats. Single-call
// dashboard rollup: distinct device count + ranked breakdowns of
// clients, MCP servers, and skills computed over each device's
// latest scan in the window. Default window is the last 60 days
// when no start/end is supplied. Admin / owner / auditor only.
func (*DeviceScansHandler) GetScanStats(req api.Context) error {
	q := req.URL.Query()
	end := time.Now()
	start := end.Add(-dashboardWindowDefault)
	if v := q.Get("start"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return types.NewErrBadRequest("invalid start: %v", err)
		}
		start = t
	}
	if v := q.Get("end"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return types.NewErrBadRequest("invalid end: %v", err)
		}
		end = t
	}

	stats, err := req.GatewayClient.GetDeviceScanStats(req.Context(), gateway.DeviceScanStatsOptions{
		StartTime: start,
		EndTime:   end,
	})
	if err != nil {
		return err
	}

	out := types.DeviceScanStats{
		TimeStart:      *types.NewTime(start),
		TimeEnd:        *types.NewTime(end),
		DeviceCount:    stats.DeviceCount,
		UserCount:      stats.UserCount,
		Clients:        make([]types.DeviceClientStat, 0, len(stats.Clients)),
		MCPServers:     make([]types.DeviceMCPServerStat, 0, len(stats.MCPServers)),
		Skills:         make([]types.DeviceSkillStat, 0, len(stats.Skills)),
		ScanTimestamps: make([]types.Time, 0, len(stats.ScanTimestamps)),
	}
	for _, c := range stats.Clients {
		out.Clients = append(out.Clients, types.DeviceClientStat{
			Name:             c.Name,
			DeviceCount:      c.DeviceCount,
			UserCount:        c.UserCount,
			ObservationCount: c.ObservationCount,
		})
	}
	for _, m := range stats.MCPServers {
		out.MCPServers = append(out.MCPServers, convertMCPServerStat(m))
	}
	for _, s := range stats.Skills {
		out.Skills = append(out.Skills, types.DeviceSkillStat{
			Name:             s.Name,
			DeviceCount:      s.DeviceCount,
			UserCount:        s.UserCount,
			ObservationCount: s.ObservationCount,
		})
	}
	for _, t := range stats.ScanTimestamps {
		out.ScanTimestamps = append(out.ScanTimestamps, *types.NewTime(t))
	}
	return req.Write(out)
}

func convertMCPServerStat(r gtypes.MCPServerStat) types.DeviceMCPServerStat {
	return types.DeviceMCPServerStat{
		ConfigHash:       r.ConfigHash,
		Name:             r.Name,
		Transport:        r.Transport,
		Command:          r.Command,
		Args:             []string(r.Args),
		URL:              r.URL,
		DeviceCount:      r.DeviceCount,
		UserCount:        r.UserCount,
		ClientCount:      r.ClientCount,
		ObservationCount: r.ObservationCount,
	}
}

func convertMCPServerDetail(d gtypes.MCPServerDetail) types.DeviceMCPServerDetail {
	return types.DeviceMCPServerDetail{
		DeviceMCPServerStat: convertMCPServerStat(d.MCPServerStat),
		EnvKeys:             d.EnvKeys,
		HeaderKeys:          d.HeaderKeys,
	}
}

// ListClients handles GET /api/devices/clients. Paginated distinct client
// names from each device's latest scan, with users, skill metadata, and MCP
// rows attributed to that client. Optional query param `name` filters to
// client names that contain the given substring (case-insensitive on
// PostgreSQL).
func (*DeviceScansHandler) ListClients(req api.Context) error {
	q := req.URL.Query()
	limit := 100
	if v := q.Get("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil && l > 0 {
			limit = l
		}
	}
	offset := 0
	if v := q.Get("offset"); v != "" {
		if o, err := strconv.Atoi(v); err == nil && o >= 0 {
			offset = o
		}
	}
	name := strings.TrimSpace(q.Get("name"))

	rows, total, err := req.GatewayClient.ListDeviceClientFleetSummaries(req.Context(), gateway.DeviceClientFleetListOptions{
		Name:   name,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return err
	}

	items := make([]types.DeviceClientFleetSummary, 0, len(rows))
	for _, r := range rows {
		items = append(items, convertDeviceClientFleetSummary(r))
	}
	return req.Write(types.DeviceClientFleetSummaryResponse{
		DeviceClientFleetSummaryList: types.DeviceClientFleetSummaryList{Items: items},
		Total:                        total,
		Limit:                        limit,
		Offset:                       offset,
	})
}

// GetClient handles GET /api/devices/clients/{name}.
func (*DeviceScansHandler) GetClient(req api.Context) error {
	name := req.PathValue("name")
	if name == "" {
		return types.NewErrBadRequest("missing client name")
	}
	summary, err := req.GatewayClient.GetDeviceClientFleetSummary(req.Context(), name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.NewErrNotFound("client %q not found", name)
		}
		return err
	}
	return req.Write(convertDeviceClientFleetSummary(*summary))
}

func convertDeviceClientFleetSummary(r gateway.DeviceClientFleetSummary) types.DeviceClientFleetSummary {
	mcps := make([]types.DeviceMCPServerStat, len(r.MCPServers))
	for i := range r.MCPServers {
		mcps[i] = convertMCPServerStat(r.MCPServers[i])
	}
	skills := make([]types.DeviceClientFleetSkill, len(r.Skills))
	for i := range r.Skills {
		skills[i] = types.DeviceClientFleetSkill{
			Name:        r.Skills[i].Name,
			Description: r.Skills[i].Description,
			HasScripts:  r.Skills[i].HasScripts,
			Files:       r.Skills[i].Files,
		}
	}
	return types.DeviceClientFleetSummary{
		Name:       r.Name,
		Users:      r.Users,
		Skills:     skills,
		MCPServers: mcps,
	}
}
