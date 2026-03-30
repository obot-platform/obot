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

var policyViolationGroupResource = schema.GroupResource{
	Group:    "obot.obot.ai",
	Resource: "policyviolations",
}

// LogPolicyViolation encrypts sensitive fields and inserts a violation record.
func (c *Client) LogPolicyViolation(ctx context.Context, v *types.PolicyViolation) error {
	v.CreatedAt = v.CreatedAt.UTC()

	if err := c.encryptPolicyViolation(ctx, v); err != nil {
		return fmt.Errorf("failed to encrypt policy violation: %w", err)
	}

	if err := c.db.WithContext(ctx).Create(v).Error; err != nil {
		return fmt.Errorf("failed to insert policy violation: %w", err)
	}

	return nil
}

// PolicyViolationOptions represents options for querying policy violations.
type PolicyViolationOptions struct {
	UserID    []string
	PolicyID  []string
	Direction []string
	ProjectID []string
	ThreadID  []string
	Query     string
	StartTime time.Time
	EndTime   time.Time
	Limit     int
	Offset    int
	SortBy    string
	SortOrder string
}

// GetPolicyViolations retrieves policy violations with optional filters.
func (c *Client) GetPolicyViolations(ctx context.Context, opts PolicyViolationOptions) ([]types.PolicyViolation, int64, error) {
	db := c.db.WithContext(ctx).Model(&types.PolicyViolation{})

	db = applyPolicyViolationFilters(db, opts)

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

	var violations []types.PolicyViolation
	if err := db.Find(&violations).Error; err != nil {
		return nil, 0, err
	}

	return violations, total, nil
}

// GetPolicyViolation retrieves a single policy violation by ID and decrypts it.
func (c *Client) GetPolicyViolation(ctx context.Context, id uint) (*types.PolicyViolation, error) {
	var v types.PolicyViolation
	if err := c.db.WithContext(ctx).Where("id = ?", id).First(&v).Error; err != nil {
		return nil, err
	}

	if err := c.decryptPolicyViolation(ctx, &v); err != nil {
		return nil, fmt.Errorf("failed to decrypt policy violation: %w", err)
	}

	return &v, nil
}

// GetPolicyViolationFilterOptions returns distinct values for a given filter field.
func (c *Client) GetPolicyViolationFilterOptions(ctx context.Context, option string, opts PolicyViolationOptions) ([]string, error) {
	db := c.db.WithContext(ctx).Model(&types.PolicyViolation{}).Distinct(option)
	db = applyPolicyViolationFilters(db, opts)

	if opts.Limit > 0 {
		db = db.Order(option).Limit(opts.Limit)
	}

	var result []string
	return result, db.Select(option).Scan(&result).Error
}

// PolicyViolationStats holds the aggregated stats returned by GetPolicyViolationStats.
type PolicyViolationStats struct {
	ByTime      []PolicyViolationTimeBucket    `json:"byTime"`
	ByPolicy    []PolicyViolationPolicyCount   `json:"byPolicy"`
	ByUser      []PolicyViolationUserCount     `json:"byUser"`
	ByDirection PolicyViolationDirectionCounts `json:"byDirection"`
}

type PolicyViolationTimeBucket struct {
	Time  time.Time `json:"time"`
	Count int64     `json:"count"`
}

type PolicyViolationPolicyCount struct {
	PolicyID   string `json:"policyID"`
	PolicyName string `json:"policyName"`
	Count      int64  `json:"count"`
}

type PolicyViolationUserCount struct {
	UserID string `json:"userID"`
	Count  int64  `json:"count"`
}

type PolicyViolationDirectionCounts struct {
	UserMessage int64 `json:"userMessage"`
	ToolCalls   int64 `json:"toolCalls"`
}

// GetPolicyViolationStats returns aggregated statistics for policy violations.
func (c *Client) GetPolicyViolationStats(ctx context.Context, opts PolicyViolationOptions) (*PolicyViolationStats, error) {
	base := c.db.WithContext(ctx).Model(&types.PolicyViolation{})
	base = applyPolicyViolationFilters(base, opts)

	stats := &PolicyViolationStats{}

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
	stats.ByTime = c.getPolicyViolationTimeBuckets(base, opts)

	return stats, nil
}

func (c *Client) getPolicyViolationTimeBuckets(base *gorm.DB, opts PolicyViolationOptions) []PolicyViolationTimeBucket {
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

	var buckets []PolicyViolationTimeBucket
	for t := startTime.Truncate(bucketDuration); t.Before(endTime); t = t.Add(bucketDuration) {
		bucketEnd := t.Add(bucketDuration)
		var count int64
		base.Session(&gorm.Session{}).
			Where("created_at >= ? AND created_at < ?", t.UTC(), bucketEnd.UTC()).
			Count(&count)
		buckets = append(buckets, PolicyViolationTimeBucket{
			Time:  t,
			Count: count,
		})
	}

	return buckets
}

func applyPolicyViolationFilters(db *gorm.DB, opts PolicyViolationOptions) *gorm.DB {
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

func (c *Client) encryptPolicyViolation(ctx context.Context, v *types.PolicyViolation) error {
	if c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[policyViolationGroupResource]
	if transformer == nil {
		return nil
	}

	dataCtx := policyViolationDataCtx(v)
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

func (c *Client) decryptPolicyViolation(ctx context.Context, v *types.PolicyViolation) error {
	if !v.Encrypted || c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[policyViolationGroupResource]
	if transformer == nil {
		return nil
	}

	dataCtx := policyViolationDataCtx(v)
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

func policyViolationDataCtx(v *types.PolicyViolation) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s/%s/%s", policyViolationGroupResource.String(), v.PolicyID, v.UserID))
}
