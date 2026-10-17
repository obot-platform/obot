package client

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

const (
	// tokenUsageStatsTopModels is how many models each top-models list has.
	tokenUsageStatsTopModels = 10
	// tokenUsageStatsMaxBuckets is the most buckets a time histogram has.
	tokenUsageStatsMaxBuckets = 120

	// tokenUsageStatsColumns are the totals of a group of token usage records. Its one argument is
	// the successful outcome. Durations are averaged over the records that have one.
	tokenUsageStatsColumns = "COUNT(*) AS requests, " +
		"SUM(CASE WHEN outcome = ? OR outcome = '' OR outcome IS NULL THEN 1 ELSE 0 END) AS successes, " +
		"SUM(CASE WHEN fallback THEN 1 ELSE 0 END) AS fallbacks, " +
		"AVG(CASE WHEN duration_ms > 0 THEN duration_ms END) AS avg_duration_ms, " +
		"COALESCE(SUM(input_tokens), 0) AS input_tokens, " +
		"COALESCE(SUM(output_tokens), 0) AS output_tokens, " +
		"COALESCE(SUM(total_tokens), 0) AS total_tokens, " +
		"COALESCE(SUM(total_spend), 0) AS total_spend"
)

var (
	// tokenUsageStatsBucketSizes are the widths a time histogram's buckets can have; the
	// narrowest that splits the range into fewer than tokenUsageStatsMaxBuckets is used.
	tokenUsageStatsBucketSizes = []time.Duration{
		time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute,
		time.Hour, 3 * time.Hour, 6 * time.Hour, 12 * time.Hour,
		24 * time.Hour, 7 * 24 * time.Hour, 30 * 24 * time.Hour,
	}
)

// TokenUsageStatsOptions selects the requests TokenUsageStats summarizes: those in
// [Start, End] made by any of UserIDs, with any of Models and any of APIKeyIDs, when set.
type TokenUsageStatsOptions struct {
	Start, End time.Time
	UserIDs    []string
	Models     []string
	APIKeyIDs  []uint
}

// tokenUsageStatsRow is what each aggregate query returns: the group (bucket, provider or
// model) and its totals.
type tokenUsageStatsRow struct {
	Bucket        int64
	Provider      string
	Model         string
	Requests      int64
	Successes     int64
	Fallbacks     int64
	AvgDurationMs *float64
	InputTokens   int64
	OutputTokens  int64
	TotalTokens   int64
	TotalSpend    float64
}

func (c *Client) InsertTokenUsage(ctx context.Context, activity *types.RunTokenActivity) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if activity.ID == 0 {
			return tx.Create(activity).Error
		}
		return tx.Updates(activity).Error
	})
}

func (c *Client) TokenUsageForUser(ctx context.Context, userID string, start, end time.Time) ([]types.RunTokenActivity, error) {
	var activities []types.RunTokenActivity
	return activities, withUsage(c.db.WithContext(ctx)).Where("user_id = ?", userID).Where("created_at >= ? AND created_at <= ?", start, end).Order("created_at DESC").Find(&activities).Error
}

func (c *Client) TotalTokenUsageForUser(ctx context.Context, userID string, start, end time.Time) (types.RunTokenActivity, error) {
	activity, err := c.tokenUsageByUser(ctx, userID, start, end)
	if err != nil || len(activity) == 0 {
		return types.RunTokenActivity{
			UserID: userID,
		}, err
	}

	return activity[0], nil
}

func (c *Client) TokenUsageByUser(ctx context.Context, start, end time.Time) ([]types.RunTokenActivity, error) {
	return c.tokenUsageByUser(ctx, "", start, end)
}

// TokenUsageSeriesInRange returns all individual token usage records in the time range for all users.
// Results are ordered by created_at descending.
// The range is [start, end] inclusive so that the requested end time is the last moment included.
func (c *Client) TokenUsageSeriesInRange(ctx context.Context, start, end time.Time) ([]types.RunTokenActivity, error) {
	var activities []types.RunTokenActivity
	err := withUsage(c.db.WithContext(ctx)).Where("created_at >= ? AND created_at <= ?", start, end).
		Where("user_id IS NOT NULL").
		Order("created_at DESC").
		Find(&activities).Error
	return activities, err
}

// withUsage leaves out the records of failed requests that used nothing, which are only kept
// to count requests.
func withUsage(db *gorm.DB) *gorm.DB {
	return db.Where("total_tokens > 0 OR outcome = ? OR outcome = '' OR outcome IS NULL", types.LLMAuditOutcomeSuccess)
}

func (c *Client) tokenUsageByUser(ctx context.Context, userID string, start, end time.Time) ([]types.RunTokenActivity, error) {
	var activities []types.RunTokenActivity
	db := c.db.WithContext(ctx).Model(new(types.RunTokenActivity)).
		Select("user_id, "+
			"SUM(input_tokens) as input_tokens, "+
			"SUM(cache_read_tokens) as cache_read_tokens, "+
			"SUM(cache_write_tokens) as cache_write_tokens, "+
			"SUM(output_tokens) as output_tokens, "+
			"SUM(thinking_tokens) as thinking_tokens, "+
			"SUM(total_tokens) as total_tokens, "+
			"SUM(input_spend) as input_spend, "+
			"SUM(cache_read_spend) as cache_read_spend, "+
			"SUM(cache_write_spend) as cache_write_spend, "+
			"SUM(output_spend) as output_spend, "+
			"SUM(total_spend) as total_spend").
		Where("created_at >= ? AND created_at <= ?", start, end)
	if userID != "" {
		db = db.Where("user_id = ?", userID)
	} else {
		db = db.Where("user_id IS NOT NULL")
	}
	return activities, db.Group("user_id").Scan(&activities).Error
}

func (r tokenUsageStatsRow) totals() types2.TokenUsageStatsTotals {
	t := types2.TokenUsageStatsTotals{
		Requests:     r.Requests,
		Successes:    r.Successes,
		Fallbacks:    r.Fallbacks,
		InputTokens:  r.InputTokens,
		OutputTokens: r.OutputTokens,
		TotalTokens:  r.TotalTokens,
		TotalSpend:   r.TotalSpend,
	}
	if r.AvgDurationMs != nil {
		t.AvgDurationMs = *r.AvgDurationMs
	}
	return t
}

// tokenUsageStatsBucketSize is the bucket width for a time histogram of a range.
func tokenUsageStatsBucketSize(start, end time.Time) time.Duration {
	span := end.Sub(start)
	for _, size := range tokenUsageStatsBucketSizes {
		if span/size < tokenUsageStatsMaxBuckets {
			return size
		}
	}
	return tokenUsageStatsBucketSizes[len(tokenUsageStatsBucketSizes)-1]
}

// TokenUsageStats summarizes the LLM requests in a time range from their token usage records:
// totals, a time histogram, spend by provider and the top models. Each is one aggregate query.
// Records made before requests' outcomes were recorded were all successful requests.
func (c *Client) TokenUsageStats(ctx context.Context, opts TokenUsageStatsOptions) (types2.TokenUsageStats, error) {
	start, end := opts.Start.UTC(), opts.End.UTC()
	if !end.After(start) {
		return types2.TokenUsageStats{}, fmt.Errorf("the end of the range must be after its start")
	}
	bucketSize := tokenUsageStatsBucketSize(start, end)
	stats := types2.TokenUsageStats{
		Start:         *types2.NewTime(start),
		End:           *types2.NewTime(end),
		BucketSeconds: int64(bucketSize.Seconds()),
	}

	query := func() *gorm.DB {
		db := c.db.WithContext(ctx).Model(new(types.RunTokenActivity)).
			Where("created_at >= ? AND created_at <= ?", start, end).
			Where("user_id IS NOT NULL")
		if len(opts.UserIDs) > 0 {
			db = db.Where("user_id IN ?", opts.UserIDs)
		}
		if len(opts.Models) > 0 {
			db = db.Where("model IN ?", opts.Models)
		}
		if len(opts.APIKeyIDs) > 0 {
			db = db.Where("api_key_id IN ?", opts.APIKeyIDs)
		}
		return db
	}

	var totals []tokenUsageStatsRow
	if err := query().Select(tokenUsageStatsColumns, types.LLMAuditOutcomeSuccess).Scan(&totals).Error; err != nil {
		return stats, err
	}
	if len(totals) > 0 {
		stats.Totals = totals[0].totals()
	}

	var buckets []tokenUsageStatsRow
	db := query()
	bucket := tokenUsageStatsBucketExpression(db.Name())
	if err := db.Select(bucket+" AS bucket, "+tokenUsageStatsColumns, start.Unix(), int64(bucketSize.Seconds()), types.LLMAuditOutcomeSuccess).
		Group("bucket").Scan(&buckets).Error; err != nil {
		return stats, err
	}
	byBucket := make(map[int64]tokenUsageStatsRow, len(buckets))
	for _, b := range buckets {
		byBucket[b.Bucket] = b
	}
	for i, t := int64(0), start; !t.After(end); i, t = i+1, t.Add(bucketSize) {
		stats.Buckets = append(stats.Buckets, types2.TokenUsageStatsBucket{
			Time:                  *types2.NewTime(t),
			TokenUsageStatsTotals: byBucket[i].totals(),
		})
	}

	var providers []tokenUsageStatsRow
	if err := query().Select("provider, "+tokenUsageStatsColumns, types.LLMAuditOutcomeSuccess).
		Group("provider").Scan(&providers).Error; err != nil {
		return stats, err
	}
	for _, p := range providers {
		stats.Providers = append(stats.Providers, types2.TokenUsageStatsProvider{Provider: p.Provider, TokenUsageStatsTotals: p.totals()})
	}
	slices.SortFunc(stats.Providers, func(a, b types2.TokenUsageStatsProvider) int {
		return cmp.Or(cmp.Compare(b.TotalSpend, a.TotalSpend), cmp.Compare(b.Requests, a.Requests), strings.Compare(a.Provider, b.Provider))
	})

	// The models in a range are few, so they are ranked here rather than queried twice.
	var models []tokenUsageStatsRow
	if err := query().Select("model, "+tokenUsageStatsColumns, types.LLMAuditOutcomeSuccess).
		Group("model").Scan(&models).Error; err != nil {
		return stats, err
	}
	// A model's requests can be served by several providers, as when a fallback serves some
	// of them; a model is shown with the provider that served most of them.
	var modelProviders []tokenUsageStatsRow
	if err := query().Select("model, provider, COUNT(*) AS requests").
		Group("model, provider").Scan(&modelProviders).Error; err != nil {
		return stats, err
	}
	all := make([]types2.TokenUsageStatsModel, 0, len(models))
	for _, m := range models {
		all = append(all, types2.TokenUsageStatsModel{Model: m.Model, Provider: mainProvider(m.Model, modelProviders), TokenUsageStatsTotals: m.totals()})
	}
	top := func(compare func(a, b types2.TokenUsageStatsModel) int) []types2.TokenUsageStatsModel {
		sorted := slices.Clone(all)
		slices.SortFunc(sorted, func(a, b types2.TokenUsageStatsModel) int {
			return cmp.Or(compare(a, b), strings.Compare(a.Model, b.Model))
		})
		return sorted[:min(len(sorted), tokenUsageStatsTopModels)]
	}
	stats.TopModelsByRequests = top(func(a, b types2.TokenUsageStatsModel) int {
		return cmp.Or(cmp.Compare(b.Requests, a.Requests), cmp.Compare(b.TotalSpend, a.TotalSpend))
	})
	stats.TopModelsBySpend = top(func(a, b types2.TokenUsageStatsModel) int {
		return cmp.Or(cmp.Compare(b.TotalSpend, a.TotalSpend), cmp.Compare(b.Requests, a.Requests))
	})
	return stats, nil
}

// mainProvider returns the provider that served most of a model's requests, given the
// requests each provider served of each model. A tie goes to the provider the model is named
// for ("anthropic" for anthropic/claude-sonnet-4-5), then to the first in alphabetical order.
// Records without a provider count for the provider the model is named for.
func mainProvider(model string, served []tokenUsageStatsRow) string {
	named, _, _ := strings.Cut(model, "/")
	counts := map[string]int64{}
	for _, row := range served {
		if row.Model == model {
			counts[cmp.Or(row.Provider, named)] += row.Requests
		}
	}
	best := named
	for provider, n := range counts {
		if n > counts[best] || n == counts[best] && best != named && provider < best {
			best = provider
		}
	}
	return best
}

// tokenUsageStatsBucketExpression numbers the histogram bucket of a record's created_at,
// counting from 0, given the start of the range and the bucket width, in seconds.
func tokenUsageStatsBucketExpression(dialect string) string {
	if dialect == "postgres" {
		return "CAST(FLOOR((EXTRACT(EPOCH FROM created_at) - ?) / ?) AS BIGINT)"
	}
	return "((CAST(strftime('%s', created_at) AS INTEGER) - ?) / ?)"
}
