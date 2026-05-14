package client

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

// InsertDeviceScan persists a device scan envelope and all its children
// in a single GORM cascading insert. Each call creates a fresh row —
// duplicate submissions are not deduped at this layer.
func (c *Client) InsertDeviceScan(ctx context.Context, scan *types.DeviceScan) error {
	if scan == nil {
		return errors.New("nil device scan")
	}
	if err := c.db.WithContext(ctx).Create(scan).Error; err != nil {
		return fmt.Errorf("failed to insert device scan: %w", err)
	}
	return nil
}

// GetDeviceScan loads a single scan with all children preloaded.
func (c *Client) GetDeviceScan(ctx context.Context, id uint) (*types.DeviceScan, error) {
	var s types.DeviceScan
	if err := c.db.WithContext(ctx).
		Preload("MCPServers").
		Preload("Skills").
		Preload("Plugins").
		Preload("Files").
		Preload("Clients").
		First(&s, id).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// DeleteDeviceScan removes a scan and its child rows. Idempotent:
// returns nil when no scan with that id exists.
func (c *Client) DeleteDeviceScan(ctx context.Context, id uint) error {
	return c.db.WithContext(ctx).Delete(&types.DeviceScan{}, id).Error
}

// DeviceScanListOptions filters the scan-envelope list endpoint.
// SubmittedBy and DeviceID are multi-value; either narrows the result.
type DeviceScanListOptions struct {
	SubmittedBy   []string
	DeviceID      []string
	Limit         int
	Offset        int
	GroupByDevice bool
}

// ListDeviceScans returns scan envelopes ordered newest first.
// MCP servers, skills, and plugins are preloaded; files are not —
// DeviceScanFile.Content can be large and isn't needed for the list.
func (c *Client) ListDeviceScans(ctx context.Context, opts DeviceScanListOptions) ([]types.DeviceScan, int64, error) {
	db := c.db.WithContext(ctx).Model(&types.DeviceScan{})
	db = applyDeviceScanListFilters(db, opts)

	if opts.GroupByDevice {
		sub := applyDeviceScanListFilters(
			c.db.WithContext(ctx).Model(&types.DeviceScan{}).Select("MAX(id)"),
			opts,
		).Group("device_id")
		db = db.Where("id IN (?)", sub)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if opts.Limit > 0 {
		db = db.Limit(opts.Limit)
	}
	if opts.Offset > 0 {
		db = db.Offset(opts.Offset)
	}

	var scans []types.DeviceScan
	if err := db.Order("created_at DESC").
		Preload("MCPServers").
		Preload("Skills").
		Preload("Plugins").
		Preload("Clients").
		Find(&scans).Error; err != nil {
		return nil, 0, err
	}
	return scans, total, nil
}

func applyDeviceScanListFilters(db *gorm.DB, opts DeviceScanListOptions) *gorm.DB {
	if len(opts.SubmittedBy) > 0 {
		db = db.Where("submitted_by IN (?)", opts.SubmittedBy)
	}
	if len(opts.DeviceID) > 0 {
		db = db.Where("device_id IN (?)", opts.DeviceID)
	}
	return db
}

// DeviceScanStatsOptions bounds the dashboard rollup. Zero-valued
// times are treated as unbounded; callers normally pass a recent
// window (e.g. last 60 days).
type DeviceScanStatsOptions struct {
	StartTime time.Time
	EndTime   time.Time
}

// GetDeviceScanStats returns the dashboard rollup for a window: the
// distinct device count and three ranked breakdowns (clients, MCP
// servers, skills) computed over each device's latest scan in the
// window. Returns every group; the caller picks any top-N.
func (c *Client) GetDeviceScanStats(ctx context.Context, opts DeviceScanStatsOptions) (*DeviceScanStatsResult, error) {
	out := &DeviceScanStatsResult{StartTime: opts.StartTime, EndTime: opts.EndTime}

	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		latest := tx.Model(&types.DeviceScan{}).Select("MAX(id)")
		if !opts.StartTime.IsZero() {
			latest = latest.Where("scanned_at >= ?", opts.StartTime)
		}
		if !opts.EndTime.IsZero() {
			latest = latest.Where("scanned_at < ?", opts.EndTime)
		}
		latest = latest.Group("device_id")

		// Device count — number of devices with a scan in the window.
		// Equal to the row count of the latest-per-device subset.
		if err := tx.Table("device_scans").
			Where("id IN (?)", latest).
			Count(&out.DeviceCount).Error; err != nil {
			return fmt.Errorf("count devices: %w", err)
		}

		// User count — distinct submitted_by across the same subset.
		// One submitter may own multiple devices, so it's typically
		// <= device_count.
		if err := tx.Table("device_scans").
			Where("id IN (?)", latest).
			Where("submitted_by <> ''").
			Distinct("submitted_by").
			Count(&out.UserCount).Error; err != nil {
			return fmt.Errorf("count users: %w", err)
		}

		// Clients: GROUP BY name across the latest-scan-per-device subset.
		if err := tx.Table("device_scan_clients AS cl").
			Joins("JOIN device_scans AS s ON s.id = cl.device_scan_id").
			Where("s.id IN (?)", latest).
			Where("cl.name <> ''").
			Select(`cl.name AS name,
				COUNT(DISTINCT s.device_id) AS device_count,
				COUNT(DISTINCT s.submitted_by) AS user_count,
				COUNT(*) AS observation_count`).
			Group("cl.name").
			Order("device_count DESC, cl.name ASC").
			Scan(&out.Clients).Error; err != nil {
			return fmt.Errorf("aggregate clients: %w", err)
		}

		// MCP servers: GROUP BY config_hash. Args is omitted because JSONB
		// has no MAX() in Postgres and the dashboard doesn't need it.
		if err := tx.Table("device_scan_mcp_servers AS m").
			Joins("JOIN device_scans AS s ON s.id = m.device_scan_id").
			Where("s.id IN (?)", latest).
			Select(`m.config_hash AS config_hash,
				MAX(m.name) AS name,
				MAX(m.transport) AS transport,
				MAX(m.command) AS command,
				MAX(m.url) AS url,
				COUNT(DISTINCT s.device_id) AS device_count,
				COUNT(DISTINCT s.submitted_by) AS user_count,
				COUNT(DISTINCT m.client) AS client_count,
				COUNT(*) AS observation_count`).
			Group("m.config_hash").
			Order("device_count DESC, m.config_hash ASC").
			Scan(&out.MCPServers).Error; err != nil {
			return fmt.Errorf("aggregate mcp servers: %w", err)
		}

		// Skills: GROUP BY name. Same client-attribution semantics as
		// the rest of the scan: free-floating SKILL.md emits as
		// client="multi" but is grouped strictly by name here so a
		// skill named brainstorming collapses across owners.
		if err := tx.Table("device_scan_skills AS sk").
			Joins("JOIN device_scans AS s ON s.id = sk.device_scan_id").
			Where("s.id IN (?)", latest).
			Where("sk.name <> ''").
			Select(`sk.name AS name,
				COUNT(DISTINCT s.device_id) AS device_count,
				COUNT(DISTINCT s.submitted_by) AS user_count,
				COUNT(*) AS observation_count`).
			Group("sk.name").
			Order("device_count DESC, sk.name ASC").
			Scan(&out.Skills).Error; err != nil {
			return fmt.Errorf("aggregate skills: %w", err)
		}

		// Scans-over-time. Returns raw scanned_at values for every
		// submission in the window so the dashboard chart can bucket
		// them in the user's local timezone (StackedTimeline does its
		// own client-side rounding, so any server-side bucketing would
		// be re-rounded and produce off-by-tz boundaries). The data
		// volume is small — even a busy fleet over the 60-day default
		// is well under a megabyte of timestamps.
		q := tx.Model(&types.DeviceScan{})
		if !opts.StartTime.IsZero() {
			q = q.Where("scanned_at >= ?", opts.StartTime)
		}
		if !opts.EndTime.IsZero() {
			q = q.Where("scanned_at < ?", opts.EndTime)
		}
		if err := q.Order("scanned_at ASC").Pluck("scanned_at", &out.ScanTimestamps).Error; err != nil {
			return fmt.Errorf("load scan timestamps: %w", err)
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return out, nil
}

// DeviceScanStatsResult is the dashboard rollup payload.
type DeviceScanStatsResult struct {
	StartTime      time.Time
	EndTime        time.Time
	DeviceCount    int64
	UserCount      int64
	Clients        []types.ClientStat
	MCPServers     []types.MCPServerStat
	Skills         []types.SkillStat
	ScanTimestamps []time.Time
}

// GetMCPServerDetail returns the aggregated row keyed by config_hash
// plus the union of EnvKeys / HeaderKeys observed across the canonical
// rows. The aggregation is unbounded (all-time, all latest scans per
// device). Args is pulled from a canonical row (constant within a
// hash group, but JSONB has no MAX() in Postgres so it can't be
// selected with the GROUP BY).
func (c *Client) GetMCPServerDetail(ctx context.Context, configHash string) (*types.MCPServerDetail, error) {
	if configHash == "" {
		return nil, errors.New("empty config hash")
	}
	db := c.db.WithContext(ctx)

	latest := db.Model(&types.DeviceScan{}).Select("MAX(id)").Group("device_id")

	var agg types.MCPServerStat
	row := db.Table("device_scan_mcp_servers AS m").
		Joins("JOIN device_scans AS s ON s.id = m.device_scan_id").
		Where("s.id IN (?)", latest).
		Where("m.config_hash = ?", configHash).
		Select(`m.config_hash AS config_hash,
			MAX(m.name) AS name,
			MAX(m.transport) AS transport,
			MAX(m.command) AS command,
			MAX(m.url) AS url,
			COUNT(DISTINCT s.device_id) AS device_count,
			COUNT(DISTINCT s.submitted_by) AS user_count,
			COUNT(DISTINCT m.client) AS client_count,
			COUNT(*) AS observation_count`).
		Group("m.config_hash").
		Scan(&agg)
	if row.Error != nil {
		return nil, fmt.Errorf("failed to load aggregated mcp server: %w", row.Error)
	}
	if row.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	// Pull Args from a canonical row, and union EnvKeys / HeaderKeys
	// across every observation of this hash (those keys are not in the
	// hash, so they may differ per row).
	var canonical []types.DeviceScanMCPServer
	if err := db.
		Where("config_hash = ?", configHash).
		Where("device_scan_id IN (?)", latest).
		Find(&canonical).Error; err != nil {
		return nil, fmt.Errorf("failed to load mcp server canonical rows: %w", err)
	}

	out := &types.MCPServerDetail{MCPServerStat: agg}
	if len(canonical) > 0 {
		out.Args = canonical[0].Args
		envSeen := map[string]struct{}{}
		hdrSeen := map[string]struct{}{}
		for _, r := range canonical {
			for _, k := range r.EnvKeys {
				if _, ok := envSeen[k]; ok {
					continue
				}
				envSeen[k] = struct{}{}
				out.EnvKeys = append(out.EnvKeys, k)
			}
			for _, k := range r.HeaderKeys {
				if _, ok := hdrSeen[k]; ok {
					continue
				}
				hdrSeen[k] = struct{}{}
				out.HeaderKeys = append(out.HeaderKeys, k)
			}
		}
	}
	return out, nil
}

// GetSkillDetail returns the full per-skill payload for the dashboard
// drill-down: aggregated counts plus representative Description /
// HasScripts / GitRemoteURL / Files from one canonical row in the
// latest-scan-per-device subset. The aggregation is unbounded
// (all-time, all latest scans per device), matching the per-hash MCP
// detail's semantics.
func (c *Client) GetSkillDetail(ctx context.Context, name string) (*types.SkillDetail, error) {
	if name == "" {
		return nil, errors.New("empty skill name")
	}
	db := c.db.WithContext(ctx)
	latest := db.Model(&types.DeviceScan{}).Select("MAX(id)").Group("device_id")

	var stat types.SkillStat
	row := db.Table("device_scan_skills AS sk").
		Joins("JOIN device_scans AS s ON s.id = sk.device_scan_id").
		Where("s.id IN (?)", latest).
		Where("sk.name = ?", name).
		Select(`sk.name AS name,
			COUNT(DISTINCT s.device_id) AS device_count,
			COUNT(DISTINCT s.submitted_by) AS user_count,
			COUNT(*) AS observation_count`).
		Group("sk.name").
		Scan(&stat)
	if row.Error != nil {
		return nil, fmt.Errorf("failed to load skill stat: %w", row.Error)
	}
	if row.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	// Pull a canonical row for Description / HasScripts / GitRemoteURL
	// / Files. Sort by id ASC for determinism.
	var canonical types.DeviceScanSkill
	if err := db.
		Where("name = ?", name).
		Where("device_scan_id IN (?)", latest).
		Order("id ASC").
		First(&canonical).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to load canonical skill row: %w", err)
	}

	return &types.SkillDetail{
		SkillStat:    stat,
		Description:  canonical.Description,
		HasScripts:   canonical.HasScripts,
		GitRemoteURL: canonical.GitRemoteURL,
		Files:        []string(canonical.Files),
	}, nil
}

// ListSkillOccurrences returns one row per (device, observation) for
// the given skill name, drawn from the all-time latest scan of every
// device. Sorted scanned_at DESC, paginated.
func (c *Client) ListSkillOccurrences(ctx context.Context, name string, limit, offset int) ([]types.SkillOccurrence, int64, error) {
	if name == "" {
		return nil, 0, errors.New("empty skill name")
	}
	db := c.db.WithContext(ctx)
	latest := db.Model(&types.DeviceScan{}).Select("MAX(id)").Group("device_id")

	base := db.Table("device_scan_skills AS sk").
		Joins("JOIN device_scans AS s ON s.id = sk.device_scan_id").
		Where("sk.name = ?", name).
		Where("s.id IN (?)", latest)

	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count skill occurrences: %w", err)
	}

	q := base.Session(&gorm.Session{}).
		Select(`sk.id AS id,
			sk.device_scan_id AS device_scan_id,
			s.device_id AS device_id,
			sk.client AS client,
			sk.scope AS scope,
			sk.project_path AS project_path,
			s.scanned_at AS scanned_at`).
		Order("s.scanned_at DESC, sk.id ASC")

	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}

	var rows []types.SkillOccurrence
	if err := q.Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list skill occurrences: %w", err)
	}
	return rows, total, nil
}

// SkillStatListOptions filters and orders the paginated skill stats
// list. The time window applies to the parent device_scans (only
// scans inside the window are candidates for "latest per device"
// selection). Zero-valued bounds are treated as unbounded.
type SkillStatListOptions struct {
	StartTime time.Time
	EndTime   time.Time
	Name      string // case-insensitive LIKE match against skill name
	SortBy    string // name | device_count | user_count | observation_count
	SortOrder string // asc | desc
	Limit     int
	Offset    int
}

var skillStatSortColumns = map[string]string{
	"name":              "name",
	"device_count":      "device_count",
	"user_count":        "user_count",
	"observation_count": "observation_count",
}

// ListSkillStats returns one row per distinct skill name observed in
// the latest scan of any device within the requested window.
// Paginated, sortable, optional name LIKE filter.
func (c *Client) ListSkillStats(ctx context.Context, opts SkillStatListOptions) ([]types.SkillStat, int64, error) {
	db := c.db.WithContext(ctx)
	latest := db.Model(&types.DeviceScan{}).Select("MAX(id)")
	if !opts.StartTime.IsZero() {
		latest = latest.Where("scanned_at >= ?", opts.StartTime)
	}
	if !opts.EndTime.IsZero() {
		latest = latest.Where("scanned_at < ?", opts.EndTime)
	}
	latest = latest.Group("device_id")

	base := db.Table("device_scan_skills AS sk").
		Joins("JOIN device_scans AS s ON s.id = sk.device_scan_id").
		Where("s.id IN (?)", latest).
		Where("sk.name <> ''")

	if opts.Name != "" {
		like := "LIKE"
		if db.Name() == "postgres" {
			like = "ILIKE"
		}
		base = base.Where(fmt.Sprintf("sk.name %s ?", like), "%"+opts.Name+"%")
	}

	var total int64
	if err := base.Session(&gorm.Session{}).
		Distinct("sk.name").
		Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count skill stats: %w", err)
	}

	sortCol := skillStatSortColumns[opts.SortBy]
	if sortCol == "" {
		sortCol = "device_count"
	}
	sortDir := "DESC"
	if strings.EqualFold(opts.SortOrder, "asc") {
		sortDir = "ASC"
	}

	q := base.Session(&gorm.Session{}).
		Select(`sk.name AS name,
			COUNT(DISTINCT s.device_id) AS device_count,
			COUNT(DISTINCT s.submitted_by) AS user_count,
			COUNT(*) AS observation_count`).
		Group("sk.name").
		Order(fmt.Sprintf("%s %s, sk.name ASC", sortCol, sortDir))

	if opts.Limit > 0 {
		q = q.Limit(opts.Limit)
	}
	if opts.Offset > 0 {
		q = q.Offset(opts.Offset)
	}

	var rows []types.SkillStat
	if err := q.Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to aggregate skill stats: %w", err)
	}
	return rows, total, nil
}

// ListMCPServerOccurrences returns one row per (device, observation)
// for the given config_hash, drawn from the all-time latest scan of
// every device. Sorted scanned_at DESC, paginated.
func (c *Client) ListMCPServerOccurrences(ctx context.Context, configHash string, limit, offset int) ([]types.MCPServerOccurrence, int64, error) {
	if configHash == "" {
		return nil, 0, errors.New("empty config hash")
	}
	db := c.db.WithContext(ctx)

	latest := db.Model(&types.DeviceScan{}).Select("MAX(id)").Group("device_id")

	base := db.Table("device_scan_mcp_servers AS m").
		Joins("JOIN device_scans AS s ON s.id = m.device_scan_id").
		Where("m.config_hash = ?", configHash).
		Where("s.id IN (?)", latest)

	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count occurrences: %w", err)
	}

	q := base.Session(&gorm.Session{}).
		Select(`m.id AS id,
			m.device_scan_id AS device_scan_id,
			s.device_id AS device_id,
			m.client AS client,
			m.scope AS scope,
			s.scanned_at AS scanned_at`).
		Order("s.scanned_at DESC, m.id ASC")

	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}

	var rows []types.MCPServerOccurrence
	if err := q.Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list occurrences: %w", err)
	}
	return rows, total, nil
}

// DeviceClientFleetSkill is gateway-layer skill metadata for client summaries.
type DeviceClientFleetSkill struct {
	Name        string
	Description string
	HasScripts  bool
	Files       int
}

// DeviceClientFleetSummary is the gateway-layer aggregate for one client
// name; callers map it to apiclient types.
type DeviceClientFleetSummary struct {
	Name       string
	Users      []string
	Skills     []DeviceClientFleetSkill
	MCPServers []types.MCPServerStat
}

// DeviceClientFleetListOptions configures ListDeviceClientFleetSummaries.
type DeviceClientFleetListOptions struct {
	// Name, when non-empty after trimming, restricts distinct client names to
	// those matching as a case-insensitive substring (LIKE/ILIKE %Name%).
	Name string
	// Limit is the max number of client rows to return; 0 means no limit.
	Limit int
	// Offset skips that many client names in name order.
	Offset int
}

// ListDeviceClientFleetSummaries returns one row per distinct client name
// observed in device_scan_clients on each device's all-time latest scan,
// paginated by name. Each row lists distinct submitters, skills with
// metadata, and MCP servers (by config_hash) attributed to that client on
// those scans. Optional Name filters distinct names by case-insensitive
// substring match.
func (c *Client) ListDeviceClientFleetSummaries(ctx context.Context, opts DeviceClientFleetListOptions) ([]DeviceClientFleetSummary, int64, error) {
	db := c.db.WithContext(ctx)
	latest := db.Model(&types.DeviceScan{}).Select("MAX(id)").Group("device_id")

	base := db.Table("device_scan_clients AS cl").
		Joins("JOIN device_scans AS s ON s.id = cl.device_scan_id").
		Where("s.id IN (?)", latest).
		Where("cl.name <> ''")

	nameFilter := strings.TrimSpace(opts.Name)
	if nameFilter != "" {
		like := "LIKE"
		if db.Name() == "postgres" {
			like = "ILIKE"
		}
		base = base.Where(fmt.Sprintf("cl.name %s ?", like), "%"+nameFilter+"%")
	}

	var total int64
	if err := base.Session(&gorm.Session{}).Distinct("cl.name").Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count distinct clients: %w", err)
	}

	qNames := base.Session(&gorm.Session{}).
		Distinct("cl.name").
		Order("cl.name ASC")
	if opts.Limit > 0 {
		qNames = qNames.Limit(opts.Limit)
	}
	if opts.Offset > 0 {
		qNames = qNames.Offset(opts.Offset)
	}

	var names []string
	if err := qNames.Pluck("cl.name", &names).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list client names: %w", err)
	}
	if len(names) == 0 {
		return nil, total, nil
	}

	out, err := c.deviceClientFleetSummariesForNames(ctx, names)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// GetDeviceClientFleetSummary returns the aggregate for a single client
// name, or gorm.ErrRecordNotFound when that name never appears on any
// device's latest scan.
func (c *Client) GetDeviceClientFleetSummary(ctx context.Context, name string) (*DeviceClientFleetSummary, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("empty client name")
	}
	db := c.db.WithContext(ctx)
	latest := db.Model(&types.DeviceScan{}).Select("MAX(id)").Group("device_id")
	var cnt int64
	if err := db.Table("device_scan_clients AS cl").
		Joins("JOIN device_scans AS s ON s.id = cl.device_scan_id").
		Where("s.id IN (?)", latest).
		Where("cl.name = ?", name).
		Count(&cnt).Error; err != nil {
		return nil, err
	}
	if cnt == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	summaries, err := c.deviceClientFleetSummariesForNames(ctx, []string{name})
	if err != nil {
		return nil, err
	}
	if len(summaries) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &summaries[0], nil
}

func (c *Client) deviceClientFleetSummariesForNames(ctx context.Context, names []string) ([]DeviceClientFleetSummary, error) {
	db := c.db.WithContext(ctx)
	latest := db.Model(&types.DeviceScan{}).Select("MAX(id)").Group("device_id")

	type userRow struct {
		ClientName  string `gorm:"column:client_name"`
		SubmittedBy string `gorm:"column:submitted_by"`
	}
	var userRows []userRow
	if err := db.Table("device_scan_clients AS cl").
		Joins("JOIN device_scans AS s ON s.id = cl.device_scan_id").
		Where("s.id IN (?)", latest).
		Where("cl.name IN ?", names).
		Where("s.submitted_by <> ''").
		Distinct("cl.name", "s.submitted_by").
		Select("cl.name AS client_name, s.submitted_by AS submitted_by").
		Scan(&userRows).Error; err != nil {
		return nil, fmt.Errorf("failed to load client users: %w", err)
	}

	usersByClient := map[string]map[string]struct{}{}
	for _, row := range userRows {
		if usersByClient[row.ClientName] == nil {
			usersByClient[row.ClientName] = map[string]struct{}{}
		}
		usersByClient[row.ClientName][row.SubmittedBy] = struct{}{}
	}

	var skillObs []types.DeviceScanSkill
	if err := db.Table("device_scan_skills AS sk").
		Joins("JOIN device_scans AS s ON s.id = sk.device_scan_id").
		Where("s.id IN (?)", latest).
		Where("sk.client IN ?", names).
		Where("sk.client <> ?", "multi").
		Where("sk.name <> ''").
		Order("sk.client ASC, sk.name ASC, sk.id ASC").
		Find(&skillObs).Error; err != nil {
		return nil, fmt.Errorf("failed to load client skill metadata: %w", err)
	}

	skillsByClient := map[string][]DeviceClientFleetSkill{}
	seenSkillKey := map[string]map[string]struct{}{}
	for _, sk := range skillObs {
		if seenSkillKey[sk.Client] == nil {
			seenSkillKey[sk.Client] = map[string]struct{}{}
		}
		if _, dup := seenSkillKey[sk.Client][sk.Name]; dup {
			continue
		}
		seenSkillKey[sk.Client][sk.Name] = struct{}{}
		skillsByClient[sk.Client] = append(skillsByClient[sk.Client], DeviceClientFleetSkill{
			Name:        sk.Name,
			Description: sk.Description,
			HasScripts:  sk.HasScripts,
			Files:       len(sk.Files),
		})
	}
	for cl := range skillsByClient {
		slices.SortFunc(skillsByClient[cl], func(a, b DeviceClientFleetSkill) int {
			return strings.Compare(a.Name, b.Name)
		})
	}

	type mcpAggRow struct {
		Client     string `gorm:"column:client"`
		ConfigHash string `gorm:"column:config_hash"`
		Name       string `gorm:"column:name"`
		Transport  string `gorm:"column:transport"`
		Command    string `gorm:"column:command"`
		URL        string `gorm:"column:url"`
	}
	var mcpRows []mcpAggRow
	if err := db.Table("device_scan_mcp_servers AS m").
		Joins("JOIN device_scans AS s ON s.id = m.device_scan_id").
		Where("s.id IN (?)", latest).
		Where("m.client IN ?", names).
		Where("m.client <> ?", "multi").
		Select(`m.client AS client, m.config_hash AS config_hash,
			MAX(m.name) AS name, MAX(m.transport) AS transport,
			MAX(m.command) AS command, MAX(m.url) AS url`).
		Group("m.client, m.config_hash").
		Scan(&mcpRows).Error; err != nil {
		return nil, fmt.Errorf("failed to aggregate client mcp servers: %w", err)
	}

	mcpByClient := map[string][]mcpAggRow{}
	hashSet := map[string]struct{}{}
	for _, row := range mcpRows {
		mcpByClient[row.Client] = append(mcpByClient[row.Client], row)
		if row.ConfigHash != "" {
			hashSet[row.ConfigHash] = struct{}{}
		}
	}

	argsByHash := map[string][]string{}
	if len(hashSet) > 0 {
		hashes := make([]string, 0, len(hashSet))
		for h := range hashSet {
			hashes = append(hashes, h)
		}
		slices.Sort(hashes)

		var canon []types.DeviceScanMCPServer
		if err := db.
			Where("config_hash IN ?", hashes).
			Where("device_scan_id IN (?)", latest).
			Where("client IN ?", names).
			Where("client <> ?", "multi").
			Order("id ASC").
			Find(&canon).Error; err != nil {
			return nil, fmt.Errorf("failed to load canonical mcp rows: %w", err)
		}
		for _, row := range canon {
			if row.ConfigHash == "" {
				continue
			}
			if _, ok := argsByHash[row.ConfigHash]; ok {
				continue
			}
			argsByHash[row.ConfigHash] = append([]string(nil), row.Args...)
		}
	}

	out := make([]DeviceClientFleetSummary, 0, len(names))
	for _, name := range names {
		userSet := usersByClient[name]
		users := make([]string, 0, len(userSet))
		for u := range userSet {
			users = append(users, u)
		}
		slices.Sort(users)

		var mcps []types.MCPServerStat
		for _, row := range mcpByClient[name] {
			mcps = append(mcps, types.MCPServerStat{
				ConfigHash:       row.ConfigHash,
				Name:             row.Name,
				Transport:        row.Transport,
				Command:          row.Command,
				URL:              row.URL,
				Args:             argsByHash[row.ConfigHash],
				DeviceCount:      0,
				UserCount:        0,
				ClientCount:      0,
				ObservationCount: 0,
			})
		}
		slices.SortFunc(mcps, func(a, b types.MCPServerStat) int {
			return strings.Compare(a.ConfigHash, b.ConfigHash)
		})

		skills := skillsByClient[name]
		if skills == nil {
			skills = []DeviceClientFleetSkill{}
		}

		out = append(out, DeviceClientFleetSummary{
			Name:       name,
			Users:      users,
			Skills:     skills,
			MCPServers: mcps,
		})
	}
	return out, nil
}
