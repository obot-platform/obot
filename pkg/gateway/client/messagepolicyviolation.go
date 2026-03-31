package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage/value"
)

var messagePolicyViolationGroupResource = schema.GroupResource{
	Group:    "obot.obot.ai",
	Resource: "policyviolations",
}

// LogMessagePolicyViolation encrypts sensitive fields and inserts a violation record.
func (c *Client) LogMessagePolicyViolation(ctx context.Context, v *types.MessagePolicyViolation) error {
	v.CreatedAt = v.CreatedAt.UTC()

	if err := c.encryptMessagePolicyViolation(ctx, v); err != nil {
		return fmt.Errorf("failed to encrypt policy violation: %w", err)
	}

	if err := c.db.WithContext(ctx).Create(v).Error; err != nil {
		return fmt.Errorf("failed to insert policy violation: %w", err)
	}

	return nil
}

// MessagePolicyViolationOptions represents options for querying policy violations.
type MessagePolicyViolationOptions struct {
	UserID      []string
	PolicyID    []string
	Direction   []string
	ProjectID   []string
	ThreadID    []string
	Query       string
	StartTime   time.Time
	EndTime     time.Time
	Limit       int
	Offset      int
	SortBy      string
	SortOrder   string
	TimeGroupBy string // "user" or "policy" (default)
}

// GetMessagePolicyViolations retrieves policy violations with optional filters.
func (c *Client) GetMessagePolicyViolations(ctx context.Context, opts MessagePolicyViolationOptions) ([]types.MessagePolicyViolation, int64, error) {
	db := c.db.WithContext(ctx).Model(&types.MessagePolicyViolation{})

	db = applyMessagePolicyViolationFilters(db, opts)

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

	if opts.SortBy != "" {
		validSortFields := map[string]bool{
			"created_at":  true,
			"user_id":     true,
			"policy_id":   true,
			"policy_name": true,
			"direction":   true,
			"project_id":  true,
			"thread_id":   true,
		}
		if validSortFields[opts.SortBy] {
			sortOrder := "DESC"
			if opts.SortOrder == "asc" {
				sortOrder = "ASC"
			}
			db = db.Order(opts.SortBy + " " + sortOrder)
		} else {
			db = db.Order("created_at DESC")
		}
	} else {
		db = db.Order("created_at DESC")
	}

	var violations []types.MessagePolicyViolation
	if err := db.Find(&violations).Error; err != nil {
		return nil, 0, err
	}

	return violations, total, nil
}

// GetMessagePolicyViolation retrieves a single policy violation by ID and decrypts it.
func (c *Client) GetMessagePolicyViolation(ctx context.Context, id uint) (*types.MessagePolicyViolation, error) {
	var v types.MessagePolicyViolation
	if err := c.db.WithContext(ctx).Where("id = ?", id).First(&v).Error; err != nil {
		return nil, err
	}

	if err := c.decryptMessagePolicyViolation(ctx, &v); err != nil {
		return nil, fmt.Errorf("failed to decrypt policy violation: %w", err)
	}

	return &v, nil
}

// GetMessagePolicyViolationFilterOptions returns distinct values for a given filter field.
func (c *Client) GetMessagePolicyViolationFilterOptions(ctx context.Context, option string, opts MessagePolicyViolationOptions) ([]string, error) {
	db := c.db.WithContext(ctx).Model(&types.MessagePolicyViolation{}).Distinct(option)
	db = applyMessagePolicyViolationFilters(db, opts)

	if opts.Limit > 0 {
		db = db.Order(option).Limit(opts.Limit)
	}

	var result []string
	return result, db.Select(option).Scan(&result).Error
}

// MessagePolicyViolationStats holds the aggregated stats returned by GetMessagePolicyViolationStats.
type MessagePolicyViolationStats struct {
	ByTime      []MessagePolicyViolationTimeBucket    `json:"byTime"`
	ByPolicy    []MessagePolicyViolationPolicyCount   `json:"byPolicy"`
	ByUser      []MessagePolicyViolationUserCount     `json:"byUser"`
	ByDirection MessagePolicyViolationDirectionCounts `json:"byDirection"`
}

type MessagePolicyViolationTimeBucket struct {
	Time     time.Time `json:"time"`
	Category string    `json:"category"`
	Count    int64     `json:"count"`
}

type MessagePolicyViolationPolicyCount struct {
	PolicyID   string `json:"policyID"`
	PolicyName string `json:"policyName"`
	Count      int64  `json:"count"`
}

type MessagePolicyViolationUserCount struct {
	UserID string `json:"userID"`
	Count  int64  `json:"count"`
}

type MessagePolicyViolationDirectionCounts struct {
	UserMessage int64 `json:"userMessage"`
	ToolCalls   int64 `json:"toolCalls"`
}

// GetMessagePolicyViolationStats returns aggregated statistics for policy violations.
func (c *Client) GetMessagePolicyViolationStats(ctx context.Context, opts MessagePolicyViolationOptions) (*MessagePolicyViolationStats, error) {
	base := c.db.WithContext(ctx).Model(&types.MessagePolicyViolation{})
	base = applyMessagePolicyViolationFilters(base, opts)

	stats := &MessagePolicyViolationStats{}

	// by_policy
	if err := base.Session(&gorm.Session{}).
		Select("policy_id, policy_name, COUNT(*) as count").
		Group("policy_id, policy_name").
		Order("count DESC").
		Scan(&stats.ByPolicy).Error; err != nil {
		return nil, fmt.Errorf("failed to get by-policy stats: %w", err)
	}

	// by_user
	if err := base.Session(&gorm.Session{}).
		Select("user_id, COUNT(*) as count").
		Group("user_id").
		Order("count DESC").
		Scan(&stats.ByUser).Error; err != nil {
		return nil, fmt.Errorf("failed to get by-user stats: %w", err)
	}

	// by_direction
	var dirCounts []struct {
		Direction string
		Count     int64
	}
	if err := base.Session(&gorm.Session{}).
		Select("direction, COUNT(*) as count").
		Group("direction").
		Scan(&dirCounts).Error; err != nil {
		return nil, fmt.Errorf("failed to get by-direction stats: %w", err)
	}
	for _, dc := range dirCounts {
		switch dc.Direction {
		case "user-message":
			stats.ByDirection.UserMessage = dc.Count
		case "tool-calls":
			stats.ByDirection.ToolCalls = dc.Count
		}
	}

	// by_time — auto-bucket based on time range
	stats.ByTime = c.getMessagePolicyViolationTimeBuckets(base, opts)

	return stats, nil
}

func (c *Client) getMessagePolicyViolationTimeBuckets(base *gorm.DB, opts MessagePolicyViolationOptions) []MessagePolicyViolationTimeBucket {
	startTime := opts.StartTime
	endTime := opts.EndTime
	if startTime.IsZero() {
		startTime = time.Now().AddDate(0, 0, -30)
	}
	if endTime.IsZero() {
		endTime = time.Now()
	}

	duration := endTime.Sub(startTime)

	var bucketDuration time.Duration
	switch {
	case duration <= 2*time.Hour:
		bucketDuration = 5 * time.Minute
	case duration <= 24*time.Hour:
		bucketDuration = time.Hour
	case duration <= 7*24*time.Hour:
		bucketDuration = 4 * time.Hour
	case duration <= 30*24*time.Hour:
		bucketDuration = 24 * time.Hour
	default:
		bucketDuration = 7 * 24 * time.Hour
	}

	groupCol := "policy_name"
	if opts.TimeGroupBy == "user" {
		groupCol = "user_id"
	}

	var buckets []MessagePolicyViolationTimeBucket
	for t := startTime.Truncate(bucketDuration); t.Before(endTime); t = t.Add(bucketDuration) {
		bucketEnd := t.Add(bucketDuration)
		var rows []struct {
			Category string
			Count    int64
		}
		base.Session(&gorm.Session{}).
			Select(groupCol+" as category, COUNT(*) as count").
			Where("created_at >= ? AND created_at < ?", t.UTC(), bucketEnd.UTC()).
			Group(groupCol).
			Scan(&rows)
		for _, row := range rows {
			buckets = append(buckets, MessagePolicyViolationTimeBucket{
				Time:     t,
				Category: row.Category,
				Count:    row.Count,
			})
		}
	}

	return buckets
}

func applyMessagePolicyViolationFilters(db *gorm.DB, opts MessagePolicyViolationOptions) *gorm.DB {
	if opts.Query != "" {
		searchTerm := "%" + opts.Query + "%"
		like := "LIKE"
		if db.Name() == "postgres" {
			like = "ILIKE"
		}
		query := fmt.Sprintf(
			"user_id %[1]s ? OR policy_name %[1]s ? OR direction %[1]s ? OR violation_explanation %[1]s ?",
			like,
		)
		db = db.Where(query, searchTerm, searchTerm, searchTerm, searchTerm)
	}

	if len(opts.UserID) > 0 {
		db = db.Where("user_id IN (?)", opts.UserID)
	}
	if len(opts.PolicyID) > 0 {
		db = db.Where("policy_id IN (?)", opts.PolicyID)
	}
	if len(opts.Direction) > 0 {
		db = db.Where("direction IN (?)", opts.Direction)
	}
	if len(opts.ProjectID) > 0 {
		db = db.Where("project_id IN (?)", opts.ProjectID)
	}
	if len(opts.ThreadID) > 0 {
		db = db.Where("thread_id IN (?)", opts.ThreadID)
	}
	if !opts.StartTime.IsZero() {
		db = db.Where("created_at >= ?", opts.StartTime.UTC())
	}
	if !opts.EndTime.IsZero() {
		db = db.Where("created_at < ?", opts.EndTime.UTC())
	}

	return db
}

// Encryption/decryption

func (c *Client) encryptMessagePolicyViolation(ctx context.Context, v *types.MessagePolicyViolation) error {
	if c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[messagePolicyViolationGroupResource]
	if transformer == nil {
		return nil
	}

	dataCtx := messagePolicyViolationDataCtx(v)
	var errs []error

	if len(v.BlockedContent) > 0 {
		b, err := transformer.TransformToStorage(ctx, v.BlockedContent, dataCtx)
		if err != nil {
			errs = append(errs, err)
		} else {
			v.BlockedContent = json.RawMessage(base64.StdEncoding.EncodeToString(b))
		}
	}

	v.Encrypted = true
	return errors.Join(errs...)
}

func (c *Client) decryptMessagePolicyViolation(ctx context.Context, v *types.MessagePolicyViolation) error {
	if !v.Encrypted || c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[messagePolicyViolationGroupResource]
	if transformer == nil {
		return nil
	}

	dataCtx := messagePolicyViolationDataCtx(v)
	var errs []error

	if len(v.BlockedContent) > 0 {
		decoded := make([]byte, base64.StdEncoding.DecodedLen(len(v.BlockedContent)))
		n, err := base64.StdEncoding.Decode(decoded, v.BlockedContent)
		if err != nil {
			errs = append(errs, err)
		} else {
			out, _, err := transformer.TransformFromStorage(ctx, decoded[:n], dataCtx)
			if err != nil {
				errs = append(errs, err)
			} else {
				v.BlockedContent = json.RawMessage(out)
			}
		}
	}

	return errors.Join(errs...)
}

func messagePolicyViolationDataCtx(v *types.MessagePolicyViolation) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s/%s/%s", messagePolicyViolationGroupResource.String(), v.PolicyID, v.UserID))
}
