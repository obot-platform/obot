package client

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

// enforcementDecisionSortColumns is the allowlist of columns the decision-log
// list view may sort by. Anything else is rejected so sort keys can never be
// used to inject SQL.
var enforcementDecisionSortColumns = map[string]string{
	"created_at": "created_at",
	"agent":      "agent",
	"tool":       "tool",
	"kind":       "kind",
	"server":     "server_name",
	"decision":   "decision",
	"device_id":  "device_id",
	"client_ip":  "client_ip",
}

// enforcementDecisionFilterColumns maps a UI filter key to its
// storage column for both list filtering and filter-option lookups.
var enforcementDecisionFilterColumns = map[string]string{
	"agent":    "agent",
	"tool":     "tool",
	"kind":     "kind",
	"server":   "server_name",
	"decision": "decision",
	"actor":    "device_id",
}

type EnforcementDecisionOptions struct {
	MDMConfigurationID []string
	Actor              []string // device_id
	Agent              []string
	Server             []string // server_name
	Tool               []string
	Kind               []string
	Decision           []string // allow | deny

	Query     string
	StartTime time.Time
	EndTime   time.Time
	Limit     int
	Offset    int
	SortBy    string
	SortOrder string
}

var enforcementDecisionQueryColumns = []string{
	"agent", "tool", "kind", "server_name", "decision", "reason", "device_id", "client_ip",
	"server_url", "server_hostname", "server_command", "server_package_source",
	"server_package_name", "server_package_version",
}

func (o EnforcementDecisionOptions) sortExpression() (string, error) {
	sortBy := o.SortBy
	if sortBy == "" || sortBy == "timestamp" {
		return "created_at", nil
	}
	if column, ok := enforcementDecisionSortColumns[sortBy]; ok {
		return column, nil
	}
	return "", fmt.Errorf("invalid enforcement decision sort key %q", sortBy)
}

// Validate reports whether the caller-supplied sort inputs are acceptable. It is
// exported so the API layer can surface a bad sort key/direction as a 400 rather
// than letting it fall through to a 500.
func (o EnforcementDecisionOptions) Validate() error {
	if _, err := o.sortExpression(); err != nil {
		return err
	}
	if o.SortOrder != "" && o.SortOrder != "asc" && o.SortOrder != "desc" {
		return fmt.Errorf("invalid enforcement decision sort direction %q", o.SortOrder)
	}
	return nil
}

func (c *Client) applyEnforcementDecisionFilters(db *gorm.DB, opts EnforcementDecisionOptions) *gorm.DB {
	if len(opts.MDMConfigurationID) > 0 {
		db = db.Where("mdm_configuration_id IN ?", opts.MDMConfigurationID)
	}
	for _, filter := range []struct {
		column string
		values []string
	}{
		{"device_id", opts.Actor}, {"agent", opts.Agent}, {"server_name", opts.Server},
		{"tool", opts.Tool}, {"kind", opts.Kind}, {"decision", opts.Decision},
	} {
		if len(filter.values) > 0 {
			db = db.Where(filter.column+" IN ?", filter.values)
		}
	}
	if !opts.StartTime.IsZero() {
		db = db.Where("created_at >= ?", opts.StartTime.UTC())
	}
	if !opts.EndTime.IsZero() {
		db = db.Where("created_at < ?", opts.EndTime.UTC())
	}
	if opts.Query != "" {
		like := "LIKE"
		if db.Name() == "postgres" {
			like = "ILIKE"
		}
		parts := make([]string, 0, len(enforcementDecisionQueryColumns))
		args := make([]any, 0, len(enforcementDecisionQueryColumns))
		for _, column := range enforcementDecisionQueryColumns {
			parts = append(parts, column+" "+like+" ?")
			args = append(args, "%"+opts.Query+"%")
		}
		db = db.Where("("+strings.Join(parts, " OR ")+")", args...)
	}
	return db
}

func (c *Client) GetEnforcementDecisions(ctx context.Context, opts EnforcementDecisionOptions) ([]types.EnforcementDecisionLog, int64, error) {
	if err := opts.Validate(); err != nil {
		return nil, 0, err
	}
	// The sort key is valid (checked by Validate above), so the error is nil.
	sortExpression, _ := opts.sortExpression()

	db := c.applyEnforcementDecisionFilters(
		c.db.WithContext(ctx).Model(&types.EnforcementDecisionLog{}), opts)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	order := "DESC"
	if opts.SortOrder == "asc" {
		order = "ASC"
	}
	db = db.Order(sortExpression + " " + order)
	if sortExpression != "created_at" {
		db = db.Order("created_at " + order)
	}
	db = db.Order("id " + order)
	if opts.Limit > 0 {
		db = db.Limit(opts.Limit)
	}
	if opts.Offset > 0 {
		db = db.Offset(opts.Offset)
	}

	var logs []types.EnforcementDecisionLog
	if err := db.Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

func (c *Client) GetEnforcementDecision(ctx context.Context, id uint) (*types.EnforcementDecisionLog, error) {
	var decision types.EnforcementDecisionLog
	if err := c.db.WithContext(ctx).Where("id = ?", id).First(&decision).Error; err != nil {
		return nil, err
	}
	return &decision, nil
}

func (c *Client) GetEnforcementDecisionFilterOptions(ctx context.Context, option string, opts EnforcementDecisionOptions) ([]string, error) {
	column, ok := enforcementDecisionFilterColumns[option]
	if !ok {
		return nil, fmt.Errorf("invalid enforcement decision filter option %q", option)
	}

	db := c.applyEnforcementDecisionFilters(
		c.db.WithContext(ctx).Model(&types.EnforcementDecisionLog{}), opts).
		Where(column+" <> ?", "").
		Distinct(column).
		Order(column)
	if opts.Limit > 0 {
		db = db.Limit(opts.Limit)
	}

	var result []string
	return result, db.Select(column).Scan(&result).Error
}
