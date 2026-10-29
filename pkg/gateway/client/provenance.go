package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/provenance"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// provenanceRetentionBatchSize bounds one retention DELETE so a large backlog
	// is removed in steps rather than one long-running statement.
	provenanceRetentionBatchSize = 1000
	provenanceEventInsertBatch   = 100
)

var (
	// provenanceSessionSortColumns is the allowlist of columns the session list may
	// sort by. Anything else is rejected so a sort key can never reach SQL.
	provenanceSessionSortColumns = map[string]string{
		"":                 "last_activity_at",
		"last_activity_at": "last_activity_at",
		"started_at":       "started_at",
		"risk":             "risk",
	}

	_ provenance.Store = (*provenanceStore)(nil)
)

// provenanceStore implements provenance.Store over the gateway database.
type provenanceStore struct {
	c *Client
}

// ProvenanceStore returns the persistent store the provenance engine writes
// through to. Wrap it with provenance.New.
func (c *Client) ProvenanceStore() provenance.Store {
	return &provenanceStore{c: c}
}

func (s *provenanceStore) db(ctx context.Context) *gorm.DB {
	return s.c.db.WithContext(ctx)
}

// provenanceErr maps gorm's not-found to the store contract's.
func provenanceErr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return provenance.ErrNotFound
	}
	return err
}

// jsonContains builds a LIKE pattern that matches a JSON array column holding
// the string value. The value is JSON-encoded so quotes and backslashes in it
// are matched as the serializer wrote them.
func jsonContains(value string) string {
	encoded, _ := json.Marshal(value)
	return "%" + string(encoded) + "%"
}

func likeOperator(db *gorm.DB) string {
	if db.Name() == "postgres" {
		return "ILIKE"
	}
	return "LIKE"
}

// ---- sessions ----

func sessionRow(s *provenance.Session) types.ProvenanceSession {
	return types.ProvenanceSession{
		ID:             s.ID,
		UserID:         s.UserID,
		Email:          s.Email,
		Agent:          s.Agent,
		Title:          s.Title,
		StartedAt:      s.StartedAt,
		LastActivityAt: s.LastActivityAt,
		EndedAt:        s.EndedAt,
		Risk:           s.State.Risk,
		Killed:         s.State.Killed,
		LastSummary:    s.State.LastSummary,
		Labels:         s.State.Labels,
		Surfaces:       s.State.Surfaces,
		Actor:          s.Actor,
		State:          s.State,
	}
}

func rowSession(r types.ProvenanceSession) provenance.Session {
	return provenance.Session{
		ID:             r.ID,
		UserID:         r.UserID,
		Email:          r.Email,
		Agent:          r.Agent,
		Actor:          r.Actor,
		Title:          r.Title,
		StartedAt:      r.StartedAt,
		LastActivityAt: r.LastActivityAt,
		EndedAt:        r.EndedAt,
		State:          r.State,
	}
}

func (s *provenanceStore) GetSession(ctx context.Context, id string) (*provenance.Session, error) {
	var row types.ProvenanceSession
	if err := s.db(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		return nil, provenanceErr(err)
	}
	out := rowSession(row)
	return &out, nil
}

func (s *provenanceStore) ListSessions(ctx context.Context, q provenance.SessionQuery) ([]provenance.Session, int64, error) {
	sortColumn, ok := provenanceSessionSortColumns[q.SortBy]
	if !ok {
		return nil, 0, fmt.Errorf("invalid session sort key %q", q.SortBy)
	}

	db := s.db(ctx).Model(&types.ProvenanceSession{})
	if q.UserID != "" {
		db = db.Where("user_id = ?", q.UserID)
	}
	if q.Agent != "" {
		db = db.Where("agent = ?", q.Agent)
	}
	if q.Surface != "" {
		db = db.Where("surfaces LIKE ?", jsonContains(string(q.Surface)))
	}
	if q.MinRisk > 0 {
		db = db.Where("risk >= ?", q.MinRisk)
	}
	if q.Label != "" {
		db = db.Where("labels LIKE ?", jsonContains(string(q.Label)))
	}
	if q.Killed != nil {
		db = db.Where("killed = ?", *q.Killed)
	}
	if q.Open != nil {
		if *q.Open {
			db = db.Where("ended_at IS NULL")
		} else {
			db = db.Where("ended_at IS NOT NULL")
		}
	}
	if !q.Start.IsZero() {
		db = db.Where("last_activity_at >= ?", q.Start.UTC())
	}
	if !q.End.IsZero() {
		db = db.Where("started_at <= ?", q.End.UTC())
	}
	if q.Query != "" {
		like := likeOperator(db)
		pattern := "%" + q.Query + "%"
		db = db.Where(
			"(title "+like+" ? OR email "+like+" ? OR agent "+like+" ? OR last_summary "+like+" ?)",
			pattern, pattern, pattern, pattern,
		)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	order := "ASC"
	if q.SortDesc {
		order = "DESC"
	}
	db = db.Order(sortColumn + " " + order)
	if sortColumn != "last_activity_at" {
		db = db.Order("last_activity_at " + order)
	}
	db = db.Order("id " + order)
	if q.Limit > 0 {
		db = db.Limit(q.Limit)
	}
	if q.Offset > 0 {
		db = db.Offset(q.Offset)
	}

	var rows []types.ProvenanceSession
	if err := db.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]provenance.Session, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowSession(r))
	}
	return out, total, nil
}

func (s *provenanceStore) PutSession(ctx context.Context, session *provenance.Session) error {
	if session.ID == "" {
		return errors.New("session id is required")
	}
	row := sessionRow(session)
	return s.db(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		UpdateAll: true,
	}).Create(&row).Error
}

func (s *provenanceStore) ResolveLink(ctx context.Context, key string) (string, error) {
	var row types.ProvenanceSessionLink
	err := s.db(ctx).Where("key = ?", key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return row.SessionID, nil
}

func (s *provenanceStore) PutLink(ctx context.Context, key, sessionID string) error {
	row := types.ProvenanceSessionLink{Key: key, SessionID: sessionID, UpdatedAt: time.Now().UTC()}
	return s.db(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		UpdateAll: true,
	}).Create(&row).Error
}

// ---- events ----

func eventRow(ev provenance.Event) types.ProvenanceEvent {
	if ev.ID == "" {
		ev.ID = provenance.NewID("evt")
	}
	if ev.Action != nil {
		// Content never persists. The fields are already json:"-", but the
		// stored copy must not carry them either.
		a := *ev.Action
		a.Text, a.Arguments = "", nil
		ev.Action = &a
	}
	row := types.ProvenanceEvent{
		ID:        ev.ID,
		SessionID: ev.SessionID,
		UserID:    ev.UserID,
		Surface:   string(ev.Surface),
		Kind:      string(ev.Kind),
		At:        ev.At,
		ParentID:  ev.ParentID,
		Payload:   ev,
	}
	if ev.Decision != nil {
		allowed := ev.Decision.Allow
		row.Allowed = &allowed
		row.Effect = string(ev.Decision.Effect)
		row.PolicyID = ev.Decision.PolicyID
	}
	return row
}

func (s *provenanceStore) AppendEvents(ctx context.Context, events []provenance.Event) error {
	if len(events) == 0 {
		return nil
	}
	rows := make([]types.ProvenanceEvent, 0, len(events))
	for _, ev := range events {
		rows = append(rows, eventRow(ev))
	}
	return s.db(ctx).CreateInBatches(&rows, provenanceEventInsertBatch).Error
}

func (s *provenanceStore) ListEvents(ctx context.Context, q provenance.EventQuery) ([]provenance.Event, int64, error) {
	db := s.db(ctx).Model(&types.ProvenanceEvent{})
	if q.SessionID != "" {
		db = db.Where("session_id = ?", q.SessionID)
	}
	if q.UserID != "" {
		db = db.Where("user_id = ?", q.UserID)
	}
	if q.Surface != "" {
		db = db.Where("surface = ?", string(q.Surface))
	}
	if q.Kind != "" {
		db = db.Where("kind = ?", string(q.Kind))
	}
	if q.Decided != nil {
		if *q.Decided {
			db = db.Where("allowed IS NOT NULL")
		} else {
			db = db.Where("allowed IS NULL")
		}
	}
	if q.Denied != nil {
		if *q.Denied {
			db = db.Where("allowed = ?", false)
		} else {
			db = db.Where("(allowed IS NULL OR allowed = ?)", true)
		}
	}
	if !q.Start.IsZero() {
		db = db.Where(`"at" >= ?`, q.Start.UTC())
	}
	if !q.End.IsZero() {
		db = db.Where(`"at" <= ?`, q.End.UTC())
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	order := "DESC"
	if q.Ascending {
		order = "ASC"
	}
	db = db.Order(`"at" ` + order).Order("seq " + order)
	if q.Limit > 0 {
		db = db.Limit(q.Limit)
	}
	if q.Offset > 0 {
		db = db.Offset(q.Offset)
	}

	var rows []types.ProvenanceEvent
	if err := db.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]provenance.Event, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Payload)
	}
	return out, total, nil
}

func (s *provenanceStore) GetEvent(ctx context.Context, id string) (*provenance.Event, error) {
	var row types.ProvenanceEvent
	if err := s.db(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		return nil, provenanceErr(err)
	}
	ev := row.Payload
	return &ev, nil
}

// ---- policies ----

func (s *provenanceStore) ListPolicies(ctx context.Context) ([]provenance.Policy, error) {
	var rows []types.ProvenancePolicy
	if err := s.db(ctx).Order("priority DESC").Order("name ASC").Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]provenance.Policy, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Payload)
	}
	return out, nil
}

func (s *provenanceStore) GetPolicy(ctx context.Context, id string) (*provenance.Policy, error) {
	var row types.ProvenancePolicy
	if err := s.db(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		return nil, provenanceErr(err)
	}
	p := row.Payload
	return &p, nil
}

func (s *provenanceStore) PutPolicy(ctx context.Context, p *provenance.Policy) error {
	if p.ID == "" {
		p.ID = provenance.NewID("pol")
	}
	row := types.ProvenancePolicy{
		ID:        p.ID,
		Name:      p.Name,
		Effect:    string(p.Effect),
		Mode:      p.Mode,
		Enabled:   p.Enabled,
		BuiltIn:   p.BuiltIn,
		Priority:  p.Priority,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
		Payload:   *p,
	}
	return s.db(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		UpdateAll: true,
	}).Create(&row).Error
}

func (s *provenanceStore) DeletePolicy(ctx context.Context, id string) error {
	return s.db(ctx).Where("id = ?", id).Delete(&types.ProvenancePolicy{}).Error
}

// ---- approvals ----

func approvalRow(a *provenance.Approval) types.ProvenanceApproval {
	payload := *a
	payload.Action.Text, payload.Action.Arguments = "", nil
	return types.ProvenanceApproval{
		ID:        a.ID,
		Status:    a.Status,
		UserID:    a.UserID,
		SessionID: a.SessionID,
		EventID:   a.EventID,
		PolicyID:  a.PolicyID,
		CreatedAt: a.CreatedAt,
		ExpiresAt: a.ExpiresAt,
		Payload:   payload,
	}
}

func (s *provenanceStore) CreateApproval(ctx context.Context, a *provenance.Approval) error {
	if a.ID == "" {
		a.ID = provenance.NewID("apr")
	}
	row := approvalRow(a)
	return s.db(ctx).Create(&row).Error
}

func (s *provenanceStore) GetApproval(ctx context.Context, id string) (*provenance.Approval, error) {
	var row types.ProvenanceApproval
	if err := s.db(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		return nil, provenanceErr(err)
	}
	a := row.Payload
	return &a, nil
}

func (s *provenanceStore) ListApprovals(ctx context.Context, q provenance.ApprovalQuery) ([]provenance.Approval, int64, error) {
	db := s.db(ctx).Model(&types.ProvenanceApproval{})
	if q.Status != "" {
		db = db.Where("status = ?", q.Status)
	}
	if q.UserID != "" {
		db = db.Where("user_id = ?", q.UserID)
	}
	if q.SessionID != "" {
		db = db.Where("session_id = ?", q.SessionID)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	db = db.Order("created_at DESC").Order("id DESC")
	if q.Limit > 0 {
		db = db.Limit(q.Limit)
	}
	if q.Offset > 0 {
		db = db.Offset(q.Offset)
	}

	var rows []types.ProvenanceApproval
	if err := db.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]provenance.Approval, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Payload)
	}
	return out, total, nil
}

func (s *provenanceStore) UpdateApproval(ctx context.Context, a *provenance.Approval) error {
	row := approvalRow(a)
	result := s.db(ctx).Model(&types.ProvenanceApproval{}).Where("id = ?", a.ID).
		Select("status", "user_id", "session_id", "event_id", "policy_id", "created_at", "expires_at", "payload").
		Updates(&row)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return provenance.ErrNotFound
	}
	return nil
}

// ---- kills ----

func (s *provenanceStore) ListKills(ctx context.Context) ([]provenance.Kill, error) {
	var rows []types.ProvenanceKill
	if err := s.db(ctx).Order("created_at DESC").Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]provenance.Kill, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Payload)
	}
	return out, nil
}

func (s *provenanceStore) PutKill(ctx context.Context, k *provenance.Kill) error {
	if k.ID == "" {
		k.ID = provenance.NewID("kil")
	}
	row := types.ProvenanceKill{
		ID:        k.ID,
		Scope:     k.Scope,
		Target:    k.Target,
		CreatedAt: k.CreatedAt,
		Payload:   *k,
	}
	if !k.ExpiresAt.IsZero() {
		expires := k.ExpiresAt
		row.ExpiresAt = &expires
	}
	return s.db(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		UpdateAll: true,
	}).Create(&row).Error
}

func (s *provenanceStore) DeleteKill(ctx context.Context, id string) error {
	return s.db(ctx).Where("id = ?", id).Delete(&types.ProvenanceKill{}).Error
}

// ---- honeytokens ----

func (s *provenanceStore) ListHoneytokens(ctx context.Context) ([]provenance.Honeytoken, error) {
	var rows []types.ProvenanceHoneytoken
	if err := s.db(ctx).Order("created_at DESC").Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]provenance.Honeytoken, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Payload)
	}
	return out, nil
}

func (s *provenanceStore) PutHoneytoken(ctx context.Context, h *provenance.Honeytoken) error {
	if h.ID == "" {
		h.ID = provenance.NewID("hny")
	}
	row := types.ProvenanceHoneytoken{
		ID:        h.ID,
		Kind:      h.Kind,
		Name:      h.Name,
		CreatedAt: h.CreatedAt,
		Payload:   *h,
	}
	return s.db(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		UpdateAll: true,
	}).Create(&row).Error
}

func (s *provenanceStore) DeleteHoneytoken(ctx context.Context, id string) error {
	return s.db(ctx).Where("id = ?", id).Delete(&types.ProvenanceHoneytoken{}).Error
}

// ---- tool labels ----

func (s *provenanceStore) ListToolLabels(ctx context.Context) ([]provenance.ToolLabel, error) {
	var rows []types.ProvenanceToolLabel
	if err := s.db(ctx).Order("server ASC").Order("tool ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]provenance.ToolLabel, 0, len(rows))
	for _, r := range rows {
		out = append(out, provenance.ToolLabel{
			Server:    r.Server,
			Tool:      r.Tool,
			Labels:    r.Labels,
			Source:    r.Source,
			Rationale: r.Rationale,
			UpdatedAt: r.UpdatedAt,
		})
	}
	return out, nil
}

func (s *provenanceStore) PutToolLabel(ctx context.Context, l *provenance.ToolLabel) error {
	if strings.TrimSpace(l.Tool) == "" {
		return errors.New("tool name is required")
	}
	updatedAt := l.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	row := types.ProvenanceToolLabel{
		Server:    l.Server,
		Tool:      l.Tool,
		Source:    l.Source,
		Rationale: l.Rationale,
		UpdatedAt: updatedAt,
		Labels:    l.Labels,
	}
	return s.db(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "server"}, {Name: "tool"}},
		DoUpdates: clause.AssignmentColumns([]string{"source", "rationale", "updated_at", "labels"}),
	}).Create(&row).Error
}

// ---- settings ----

func (s *provenanceStore) GetSettings(ctx context.Context) (provenance.Settings, error) {
	var row types.ProvenanceSettings
	if err := s.db(ctx).Where("id = ?", types.ProvenanceSettingsID).First(&row).Error; err != nil {
		return provenance.Settings{}, provenanceErr(err)
	}
	return row.Payload, nil
}

func (s *provenanceStore) PutSettings(ctx context.Context, settings provenance.Settings) error {
	row := types.ProvenanceSettings{ID: types.ProvenanceSettingsID, UpdatedAt: time.Now().UTC(), Payload: settings}
	return s.db(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"updated_at", "payload"}),
	}).Create(&row).Error
}

// ---- retention ----

// RunProvenanceRetention deletes sessions, their events, links and approvals
// that have been idle for longer than days. Deletes run in batches so a
// large backlog never holds one long statement. Zero or negative days keeps
// everything.
func (c *Client) RunProvenanceRetention(ctx context.Context, now time.Time, days int) error {
	if days <= 0 {
		return nil
	}
	cutoff := now.UTC().Truncate(24*time.Hour).AddDate(0, 0, -days)
	for _, stmt := range []struct {
		table  string
		key    string
		column string
	}{
		{table: "provenance_events", key: "seq", column: `"at"`},
		{table: "provenance_approvals", key: "id", column: "created_at"},
		{table: "provenance_session_links", key: "key", column: "updated_at"},
		{table: "provenance_sessions", key: "id", column: "last_activity_at"},
	} {
		if err := c.deleteProvenanceBatches(ctx, stmt.table, stmt.key, stmt.column, cutoff); err != nil {
			return fmt.Errorf("provenance retention on %s: %w", stmt.table, err)
		}
	}
	return nil
}

func (c *Client) deleteProvenanceBatches(ctx context.Context, table, key, column string, cutoff time.Time) error {
	query := fmt.Sprintf(
		"DELETE FROM %s WHERE %s IN (SELECT %s FROM %s WHERE %s < ? LIMIT ?)",
		table, key, key, table, column,
	)
	for {
		result := c.db.WithContext(ctx).Exec(query, cutoff, provenanceRetentionBatchSize)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected < int64(provenanceRetentionBatchSize) {
			return nil
		}
	}
}
