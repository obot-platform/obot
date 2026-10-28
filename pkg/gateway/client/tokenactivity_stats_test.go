package client

import (
	"context"
	"math"
	"os"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

func TestTokenUsageStats(t *testing.T) {
	testTokenUsageStats(t, newTestClient(t))
}

// The histogram's buckets are numbered with each database's own date functions. Set
// OBOT_TEST_POSTGRES_DSN to run it against PostgreSQL.
func TestTokenUsageStatsPostgres(t *testing.T) {
	dsn := os.Getenv(postgresUserLimitTestDSNEnv)
	if dsn == "" {
		t.Skipf("set %s to a PostgreSQL URL whose user can create schemas", postgresUserLimitTestDSNEnv)
	}
	admin := newPostgresUserLimitTestDB(t, dsn)
	schema := "obot_token_usage_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	if err := admin.WithContext(t.Context()).Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatalf("creating isolated PostgreSQL schema: %v", err)
	}
	t.Cleanup(func() {
		if err := admin.WithContext(context.Background()).Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Errorf("dropping isolated PostgreSQL schema: %v", err)
		}
	})
	database := newPostgresUserLimitTestDB(t, postgresUserLimitTestDSN(t, dsn, schema))
	if err := database.WithContext(t.Context()).AutoMigrate(&types.RunTokenActivity{}); err != nil {
		t.Fatalf("migrating token usage: %v", err)
	}
	testTokenUsageStats(t, &Client{db: database})
}

func testTokenUsageStats(t *testing.T, c *Client) {
	t.Helper()
	ctx := t.Context()
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(6 * time.Hour)
	for _, idx := range []string{"idx_run_token_activity_created", "idx_run_token_activity_user_created"} {
		if !c.db.WithContext(ctx).Migrator().HasIndex(&types.RunTokenActivity{}, idx) {
			t.Fatalf("expected index %s", idx)
		}
	}

	keyID := uint(7)
	rows := []types.RunTokenActivity{
		{
			CreatedAt:  start.Add(10 * time.Minute),
			UserID:     "u1",
			Model:      "anthropic/claude-sonnet-4-5",
			Provider:   "anthropic",
			Outcome:    types.LLMAuditOutcomeSuccess,
			DurationMs: 100,
			APIKeyID:   &keyID,
			Usage:      types.TokenUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15, TotalSpend: 0.5},
		},
		{
			CreatedAt:  start.Add(20 * time.Minute),
			UserID:     "u1",
			Model:      "anthropic/claude-sonnet-4-5",
			Provider:   "bedrock",
			Outcome:    types.LLMAuditOutcomeSuccess,
			DurationMs: 300,
			Fallback:   true,
			Usage:      types.TokenUsage{InputTokens: 20, OutputTokens: 10, TotalTokens: 30, TotalSpend: 1},
		},
		{
			CreatedAt:  start.Add(3*time.Hour + time.Minute),
			UserID:     "u2",
			Model:      "openai/gpt-5",
			Provider:   "openai",
			Outcome:    types.LLMAuditOutcomeError,
			DurationMs: 50,
		},
		{
			// Recorded before outcomes and durations were: a successful request.
			CreatedAt: start.Add(3*time.Hour + 2*time.Minute),
			UserID:    "u2",
			Model:     "openai/gpt-5",
			Usage:     types.TokenUsage{InputTokens: 100, OutputTokens: 100, TotalTokens: 200, TotalSpend: 2},
		},
		{
			// Outside the range.
			CreatedAt: end.Add(time.Minute),
			UserID:    "u1",
			Model:     "openai/gpt-5",
			Outcome:   types.LLMAuditOutcomeSuccess,
			Usage:     types.TokenUsage{TotalTokens: 1000, TotalSpend: 100},
		},
	}
	for i := range rows {
		if err := c.InsertTokenUsage(ctx, &rows[i]); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	stats, err := c.TokenUsageStats(ctx, TokenUsageStatsOptions{Start: start, End: end})
	if err != nil {
		t.Fatalf("TokenUsageStats: %v", err)
	}
	totals := stats.Totals
	if totals.Requests != 4 || totals.Successes != 3 || totals.Fallbacks != 1 {
		t.Errorf("requests/successes/fallbacks = %d/%d/%d, want 4/3/1", totals.Requests, totals.Successes, totals.Fallbacks)
	}
	if totals.InputTokens != 130 || totals.OutputTokens != 115 || totals.TotalTokens != 245 || math.Abs(totals.TotalSpend-3.5) > 1e-9 {
		t.Errorf("usage totals = %+v", totals)
	}
	if math.Abs(totals.AvgDurationMs-150) > 1e-9 {
		t.Errorf("average duration = %v, want 150 (records without a duration are left out)", totals.AvgDurationMs)
	}

	// Three-minute buckets would make 120 of them, too many, so five-minute buckets are used.
	if stats.BucketSeconds != 300 || len(stats.Buckets) != 73 {
		t.Fatalf("bucket seconds = %d with %d buckets, want 300 with 73", stats.BucketSeconds, len(stats.Buckets))
	}
	for i, b := range stats.Buckets {
		var want int64
		switch i {
		case 2, 4, 36: // 0:10, 0:20, 3:01 and 3:02
			want = 1
			if i == 36 {
				want = 2
			}
		}
		if b.Requests != want {
			t.Errorf("bucket %d (%s) has %d requests, want %d", i, b.Time.Time, b.Requests, want)
		}
		if !b.Time.Time.Equal(start.Add(time.Duration(i) * 5 * time.Minute)) {
			t.Errorf("bucket %d starts at %s", i, b.Time.Time)
		}
	}
	if b := stats.Buckets[36]; b.Successes != 1 || b.TotalTokens != 200 {
		t.Errorf("bucket 36 = %+v, want 1 success and 200 tokens", b)
	}

	providers := map[string]float64{}
	for _, p := range stats.Providers {
		providers[p.Provider] = p.TotalSpend
	}
	if len(stats.Providers) != 4 || providers["anthropic"] != 0.5 || providers["bedrock"] != 1 || providers["openai"] != 0 || providers[""] != 2 {
		t.Errorf("providers = %+v", stats.Providers)
	}
	if stats.Providers[0].Provider != "" {
		t.Errorf("providers are not sorted by spend: %+v", stats.Providers)
	}

	if len(stats.TopModelsByRequests) != 2 || len(stats.TopModelsBySpend) != 2 {
		t.Fatalf("top models = %+v / %+v", stats.TopModelsByRequests, stats.TopModelsBySpend)
	}
	// Both have two requests; the tie goes to the one that spent more.
	if m := stats.TopModelsByRequests[0]; m.Model != "openai/gpt-5" || m.Provider != "openai" || m.Requests != 2 || m.Successes != 1 {
		t.Errorf("top model by requests = %+v", m)
	}
	if m := stats.TopModelsBySpend[0]; m.Model != "openai/gpt-5" || m.TotalSpend != 2 {
		t.Errorf("top model by spend = %+v", m)
	}
	// Anthropic and Bedrock each served one of its requests; the tie goes to Anthropic, which
	// the model is named for.
	if m := stats.TopModelsByRequests[1]; m.Model != "anthropic/claude-sonnet-4-5" || m.Provider != "anthropic" {
		t.Errorf("second model by requests = %+v, want anthropic/claude-sonnet-4-5 from anthropic", m)
	}

	filtered, err := c.TokenUsageStats(ctx, TokenUsageStatsOptions{Start: start, End: end, UserIDs: []string{"u1"}, APIKeyIDs: []uint{keyID}})
	if err != nil {
		t.Fatalf("TokenUsageStats filtered: %v", err)
	}
	if filtered.Totals.Requests != 1 || filtered.Totals.TotalTokens != 15 {
		t.Errorf("filtered totals = %+v", filtered.Totals)
	}
	filtered, err = c.TokenUsageStats(ctx, TokenUsageStatsOptions{Start: start, End: end, Models: []string{"openai/gpt-5"}})
	if err != nil {
		t.Fatalf("TokenUsageStats by model: %v", err)
	}
	if filtered.Totals.Requests != 2 || filtered.Totals.Successes != 1 {
		t.Errorf("by-model totals = %+v", filtered.Totals)
	}

	if _, err := c.TokenUsageStats(ctx, TokenUsageStatsOptions{Start: end, End: start}); err == nil {
		t.Error("expected an error for an empty range")
	}

	// The per-request usage lists leave out failed requests that used nothing.
	series, err := c.TokenUsageSeriesInRange(ctx, start, end)
	if err != nil {
		t.Fatalf("TokenUsageSeriesInRange: %v", err)
	}
	if len(series) != 3 {
		t.Errorf("series has %d records, want 3", len(series))
	}
	forUser, err := c.TokenUsageForUser(ctx, "u2", start, end)
	if err != nil {
		t.Fatalf("TokenUsageForUser: %v", err)
	}
	if len(forUser) != 1 || forUser[0].Usage.TotalTokens != 200 {
		t.Errorf("usage for u2 = %+v", forUser)
	}
}

func TestTokenUsageStatsBucketSize(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name string
		span time.Duration
		want time.Duration
	}{
		{
			name: "one hour",
			span: time.Hour,
			want: time.Minute,
		},
		{
			name: "one day",
			span: 24 * time.Hour,
			want: 15 * time.Minute,
		},
		{
			name: "one week",
			span: 7 * 24 * time.Hour,
			want: 3 * time.Hour,
		},
		{
			name: "one year",
			span: 365 * 24 * time.Hour,
			want: 7 * 24 * time.Hour,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tokenUsageStatsBucketSize(start, start.Add(tt.span)); got != tt.want {
				t.Errorf("bucket size = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestTokenUsageStatsModelProvider(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	add := func(model, provider string, fallback bool) {
		t.Helper()
		if err := c.InsertTokenUsage(ctx, &types.RunTokenActivity{
			CreatedAt: start.Add(time.Minute),
			UserID:    "u1",
			Model:     model,
			Provider:  provider,
			Fallback:  fallback,
			Outcome:   types.LLMAuditOutcomeSuccess,
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Anthropic served most of the requests; a fallback to OpenAI served the last one.
	add("anthropic/claude-haiku-4-5", "anthropic", false)
	add("anthropic/claude-haiku-4-5", "anthropic", false)
	add("anthropic/claude-haiku-4-5", "openai", true)
	// A routing rule sends every request for this model to Azure.
	add("openai/gpt-4o", "azure", false)

	stats, err := c.TokenUsageStats(ctx, TokenUsageStatsOptions{Start: start, End: start.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	providers := map[string]string{}
	for _, m := range stats.TopModelsByRequests {
		providers[m.Model] = m.Provider
	}
	if providers["anthropic/claude-haiku-4-5"] != "anthropic" || providers["openai/gpt-4o"] != "azure" {
		t.Errorf("model providers = %v, want claude-haiku-4-5 from anthropic and gpt-4o from azure", providers)
	}
}
