package client

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

func TestParseLLMGatewayBudgetWindow(t *testing.T) {
	now := time.Date(2026, 9, 24, 15, 30, 0, 0, time.UTC) // a Thursday

	tests := []struct {
		name      string
		limit     types.LLMGatewayBudgetLimit
		wantStart time.Time
		wantReset time.Time
		wantErr   bool
	}{
		{
			name:      "rolling day",
			limit:     types.LLMGatewayBudgetLimit{Window: "1d"},
			wantStart: now.AddDate(0, 0, -1),
		},
		{
			name:      "rolling six hours",
			limit:     types.LLMGatewayBudgetLimit{Window: "6h"},
			wantStart: now.Add(-6 * time.Hour),
		},
		{
			name:      "calendar day",
			limit:     types.LLMGatewayBudgetLimit{Window: "1d", Calendar: true},
			wantStart: time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC),
			wantReset: time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "calendar week starts monday",
			limit:     types.LLMGatewayBudgetLimit{Window: "1w", Calendar: true},
			wantStart: time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC),
			wantReset: time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "calendar month",
			limit:     types.LLMGatewayBudgetLimit{Window: "1M", Calendar: true},
			wantStart: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
			wantReset: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:    "multi-unit calendar windows are rejected",
			limit:   types.LLMGatewayBudgetLimit{Window: "2d", Calendar: true},
			wantErr: true,
		},
		{
			name:    "unknown unit",
			limit:   types.LLMGatewayBudgetLimit{Window: "1y"},
			wantErr: true,
		},
		{
			name:      "a year is the longest window",
			limit:     types.LLMGatewayBudgetLimit{Window: "12M"},
			wantStart: now.AddDate(-1, 0, 0),
		},
		{
			name:      "a leap year's days",
			limit:     types.LLMGatewayBudgetLimit{Window: "366d"},
			wantStart: now.AddDate(0, 0, -366),
		},
		{
			name:    "longer than a year, in months",
			limit:   types.LLMGatewayBudgetLimit{Window: "24M"},
			wantErr: true,
		},
		{
			name:    "longer than a year, in days",
			limit:   types.LLMGatewayBudgetLimit{Window: "1000d"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, reset, err := ParseLLMGatewayBudgetWindow(tt.limit, now)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !start.Equal(tt.wantStart) || !reset.Equal(tt.wantReset) {
				t.Fatalf("got start %v reset %v, want start %v reset %v", start, reset, tt.wantStart, tt.wantReset)
			}
		})
	}
}

func TestLLMGatewayBudgets(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()

	perUser := &types.LLMGatewayBudget{
		Name:      "Everyone",
		Enabled:   true,
		PerUser:   true,
		Limits:    []types.LLMGatewayBudgetLimit{{Metric: types.LLMGatewayBudgetMetricRequests, Amount: 2, Window: "1d"}},
		ProfileID: "everyone",
	}
	shared := &types.LLMGatewayBudget{
		Name:      "Engineering",
		Enabled:   true,
		Limits:    []types.LLMGatewayBudgetLimit{{Metric: types.LLMGatewayBudgetMetricSpend, Amount: 1, Window: "1M", Calendar: true}},
		ProfileID: "engineering",
	}
	opusOnly := &types.LLMGatewayBudget{
		Name:         "Opus",
		Enabled:      true,
		PerUser:      true,
		ExemptAdmins: true,
		Limits:       []types.LLMGatewayBudgetLimit{{Metric: types.LLMGatewayBudgetMetricTokens, Amount: 10, Window: "1h"}},
		Models:       []string{"m1-opus"},
		ProfileID:    "alice",
	}
	disabled := &types.LLMGatewayBudget{
		Name:      "Disabled",
		Enabled:   false,
		Limits:    []types.LLMGatewayBudgetLimit{{Metric: types.LLMGatewayBudgetMetricRequests, Amount: 0, Window: "1d"}},
		ProfileID: "everyone",
	}
	for _, b := range []*types.LLMGatewayBudget{perUser, shared, opusOnly, disabled} {
		if err := c.CreateLLMGatewayBudget(ctx, b); err != nil {
			t.Fatalf("create %s: %v", b.Name, err)
		}
	}

	names := func(bs []types.LLMGatewayBudget) []string {
		var out []string
		for _, b := range bs {
			out = append(out, b.Name)
		}
		return out
	}

	got, err := c.LLMGatewayBudgetsForUser(ctx, []string{"everyone", "engineering", "alice"}, false, "m1-opus")
	if err != nil {
		t.Fatal(err)
	}
	if n := names(got); len(n) != 3 {
		t.Fatalf("alice on opus: got %v, want Everyone, Engineering and Opus", n)
	}
	got, _ = c.LLMGatewayBudgetsForUser(ctx, []string{"everyone", "engineering", "alice"}, true, "m1-opus")
	if n := names(got); len(n) != 2 {
		t.Fatalf("admin alice should be exempt from Opus: got %v", n)
	}
	got, _ = c.LLMGatewayBudgetsForUser(ctx, []string{"everyone"}, false, "m1-gpt")
	if n := names(got); len(n) != 1 || n[0] != "Everyone" {
		t.Fatalf("bob: got %v, want only Everyone", n)
	}

	// Per-user budgets count each user separately; shared budgets count everyone.
	usage := types.TokenUsage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5, TotalSpend: 0.6}
	if err := c.RecordLLMGatewayBudgetUsage(ctx, []string{perUser.ID, shared.ID}, "alice", "m1-gpt", usage); err != nil {
		t.Fatal(err)
	}
	if err := c.CheckLLMGatewayBudgets(ctx, []types.LLMGatewayBudget{*perUser, *shared}, "alice"); err != nil {
		t.Fatalf("alice should still be within her budgets: %v", err)
	}
	if err := c.RecordLLMGatewayBudgetUsage(ctx, []string{perUser.ID, shared.ID}, "carol", "m1-gpt", usage); err != nil {
		t.Fatal(err)
	}

	var exceeded *LLMGatewayBudgetExceeded
	err = c.CheckLLMGatewayBudgets(ctx, []types.LLMGatewayBudget{*shared}, "alice")
	if !errors.As(err, &exceeded) || exceeded.Budget.Name != "Engineering" || exceeded.ResetAt.IsZero() {
		t.Fatalf("expected the shared spend budget to be exhausted, got %v", err)
	}
	if err := c.CheckLLMGatewayBudgets(ctx, []types.LLMGatewayBudget{*perUser}, "alice"); err != nil {
		t.Fatalf("carol's usage must not count against alice's own budget: %v", err)
	}

	if err := c.RecordLLMGatewayBudgetUsage(ctx, []string{perUser.ID}, "alice", "m1-gpt", usage); err != nil {
		t.Fatal(err)
	}
	err = c.CheckLLMGatewayBudgets(ctx, []types.LLMGatewayBudget{*perUser}, "alice")
	if !errors.As(err, &exceeded) || exceeded.Limit.Metric != types.LLMGatewayBudgetMetricRequests || exceeded.ResetAt.IsZero() {
		t.Fatalf("expected alice's request limit to be reached with a reset time, got %v", err)
	}

	report, err := c.LLMGatewayBudgetUsageReport(ctx, perUser)
	if err != nil {
		t.Fatal(err)
	}
	byUser := map[string]float64{}
	for _, e := range report {
		byUser[e.UserID] = e.Used[0]
	}
	if byUser["alice"] != 2 || byUser["carol"] != 1 {
		t.Fatalf("unexpected per-user report %v", byUser)
	}
	report, _ = c.LLMGatewayBudgetUsageReport(ctx, shared)
	if len(report) != 1 || report[0].Used[0] < 1.19 || report[0].Used[0] > 1.21 {
		t.Fatalf("unexpected shared report %+v", report)
	}

	// A caller in no profile has no budgets.
	if got, _ := c.LLMGatewayBudgetsForUser(ctx, nil, false, "m1-gpt"); len(got) != 0 {
		t.Fatalf("a caller in no profile should have no budgets, got %v", names(got))
	}

	// Listing by profile returns only that profile's budgets.
	listed, err := c.ListLLMGatewayBudgets(ctx, "everyone")
	if err != nil {
		t.Fatal(err)
	}
	if n := names(listed); len(n) != 2 || n[0] != "Everyone" || n[1] != "Disabled" {
		t.Fatalf("everyone's budgets: got %v", n)
	}

	// Updating moves the budget to another profile; deleting removes usage too.
	perUser.ProfileID = "dave"
	if err := c.UpdateLLMGatewayBudget(ctx, perUser); err != nil {
		t.Fatal(err)
	}
	got, _ = c.LLMGatewayBudgetsForUser(ctx, []string{"everyone"}, false, "m1-gpt")
	if len(got) != 0 {
		t.Fatalf("everyone should no longer have budgets, got %v", names(got))
	}
	if err := c.DeleteLLMGatewayBudget(ctx, perUser.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetLLMGatewayBudget(ctx, perUser.ID); err == nil {
		t.Fatal("expected the budget to be gone")
	}

	// Deleting a profile's budgets deletes them with their usage, and no other profile's.
	if err := c.DeleteLLMGatewayBudgetsForProfile(ctx, "engineering"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetLLMGatewayBudget(ctx, shared.ID); err == nil {
		t.Fatal("expected the engineering budget to be gone")
	}
	if report, _ := c.LLMGatewayBudgetUsageReport(ctx, shared); len(report) != 0 && report[0].Used[0] != 0 {
		t.Fatalf("expected the engineering budget's usage to be gone, got %+v", report)
	}
	if _, err := c.GetLLMGatewayBudget(ctx, opusOnly.ID); err != nil {
		t.Fatalf("another profile's budget was deleted: %v", err)
	}
}

func TestLLMGatewayBudgetRequiresProfile(t *testing.T) {
	c := newTestClient(t)
	err := c.CreateLLMGatewayBudget(t.Context(), &types.LLMGatewayBudget{
		Name:   "No profile",
		Limits: []types.LLMGatewayBudgetLimit{{Metric: types.LLMGatewayBudgetMetricRequests, Amount: 1, Window: "1d"}},
	})
	if err == nil || !strings.Contains(err.Error(), "profileID") {
		t.Fatalf("expected a budget without a profile to be refused, got %v", err)
	}
}

func TestWithUserTokenLimits(t *testing.T) {
	limits := []types.LLMGatewayBudgetLimit{
		{Metric: types.LLMGatewayBudgetMetricInputTokens, Amount: 1000, Window: "1d"},
		{Metric: types.LLMGatewayBudgetMetricOutputTokens, Amount: 500, Window: "1d"},
		{Metric: types.LLMGatewayBudgetMetricSpend, Amount: 5, Window: "1d"},
	}
	tests := []struct {
		name string
		user types.User
		want map[string]float64
	}{
		{
			name: "no personal limits keeps the budget's",
			want: map[string]float64{"input_tokens": 1000, "output_tokens": 500, "spend": 5},
		},
		{
			name: "a personal limit replaces the amount",
			user: types.User{DailyInputTokensLimit: 5000},
			want: map[string]float64{"input_tokens": 5000, "output_tokens": 500, "spend": 5},
		},
		{
			name: "a negative personal limit removes the limit",
			user: types.User{DailyOutputTokensLimit: -1},
			want: map[string]float64{"input_tokens": 1000, "spend": 5},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := map[string]float64{}
			for _, l := range withUserTokenLimits(limits, &tt.user) {
				got[l.Metric] = l.Amount
			}
			if !maps.Equal(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLLMGatewayBudgetsCoverOwnProfileModels(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()

	spend := []types.LLMGatewayBudgetLimit{{Metric: types.LLMGatewayBudgetMetricSpend, Amount: 10, Window: "1M", Calendar: true}}
	for _, b := range []*types.LLMGatewayBudget{
		{Name: "Everyone spend", Enabled: true, PerUser: true, Limits: spend, ProfileID: "everyone"},
		{Name: "Power spend", Enabled: true, PerUser: true, Limits: spend, ProfileID: "power"},
		{Name: "Power B", Enabled: true, PerUser: true, Limits: spend, ProfileID: "power", Models: []string{"m1-b"}},
		{Name: "Empty spend", Enabled: true, PerUser: true, Limits: spend, ProfileID: "empty"},
		{Name: "Wildcard spend", Enabled: true, PerUser: true, Limits: spend, ProfileID: "wildcard"},
	} {
		if err := c.CreateLLMGatewayBudget(ctx, b); err != nil {
			t.Fatalf("create %s: %v", b.Name, err)
		}
	}

	tests := []struct {
		name       string
		profileIDs []string
		model      string
		want       []string
	}{
		{
			name:       "model B: only Power grants it, so only Power's budgets apply",
			profileIDs: []string{"power"},
			model:      "m1-b",
			want:       []string{"Power spend", "Power B"},
		},
		{
			name:       "model A: Everyone and Power both grant it",
			profileIDs: []string{"everyone", "power"},
			model:      "m1-a",
			want:       []string{"Everyone spend", "Power spend"},
		},
		{
			name:       "a profile without models grants nothing, so its budgets never apply",
			profileIDs: nil,
			model:      "m1-c",
			want:       nil,
		},
		{
			name:       "a wildcard profile grants every model",
			profileIDs: []string{"wildcard"},
			model:      "m1-c",
			want:       []string{"Wildcard spend"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := c.LLMGatewayBudgetsForUser(ctx, tt.profileIDs, false, tt.model)
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, b := range got {
				names = append(names, b.Name)
			}
			if !slices.Equal(names, tt.want) {
				t.Fatalf("got %v, want %v", names, tt.want)
			}
		})
	}
}

func staticProfiles(ids ...string) func(context.Context) ([]string, error) {
	return func(context.Context) ([]string, error) {
		return ids, nil
	}
}

func TestCarryOverDailyTokenLimits(t *testing.T) {
	tests := []struct {
		name string
		// profiles are the profiles that exist at the upgrade.
		profiles []string
		// oldBudgetProfile, if set, has the budget an earlier build carried over to one profile.
		oldBudgetProfile string
		input            int
		output           int
		// want maps each expected budget's ID to its profile.
		want       map[string]string
		wantLimits []string
	}{
		{
			name: "several profiles get one copy each",
			profiles: []string{
				"map-default",
				"map-sales",
				"map-engineering",
			},
			input:  1000,
			output: 500,
			want: map[string]string{
				"daily-token-limit-map-default":     "map-default",
				"daily-token-limit-map-sales":       "map-sales",
				"daily-token-limit-map-engineering": "map-engineering",
			},
			wantLimits: []string{
				types.LLMGatewayBudgetMetricInputTokens,
				types.LLMGatewayBudgetMetricOutputTokens,
			},
		},
		{
			name: "a fresh install: the Default Policy EnsureProfiles creates gets the budget",
			profiles: []string{
				"map-default",
			},
			input:  1000,
			output: -1,
			want: map[string]string{
				"daily-token-limit-map-default": "map-default",
			},
			wantLimits: []string{
				types.LLMGatewayBudgetMetricInputTokens,
			},
		},
		{
			name: "the earlier single budget counts as its profile's copy",
			profiles: []string{
				"map-default",
				"map-sales",
			},
			oldBudgetProfile: "map-default",
			input:            1000,
			output:           500,
			want: map[string]string{
				"daily-token-limit":           "map-default",
				"daily-token-limit-map-sales": "map-sales",
			},
			wantLimits: []string{
				types.LLMGatewayBudgetMetricInputTokens,
				types.LLMGatewayBudgetMetricOutputTokens,
			},
		},
		{
			name: "both limits disabled: no budgets",
			profiles: []string{
				"map-default",
			},
			input:  -1,
			output: -1,
			want:   map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t)
			if tt.oldBudgetProfile != "" {
				if err := c.createLLMGatewayBudget(t.Context(), &types.LLMGatewayBudget{
					ID:        defaultLLMGatewayBudgetID,
					Name:      "Daily token limit",
					Enabled:   true,
					PerUser:   true,
					Limits:    []types.LLMGatewayBudgetLimit{{Metric: types.LLMGatewayBudgetMetricInputTokens, Amount: 1, Window: "1d"}},
					ProfileID: tt.oldBudgetProfile,
				}); err != nil {
					t.Fatal(err)
				}
			}
			// Twice, as replicas starting together would.
			for range 2 {
				if err := c.CarryOverDailyTokenLimits(t.Context(), staticProfiles(tt.profiles...), tt.input, tt.output); err != nil {
					t.Fatal(err)
				}
			}
			budgets, err := c.ListLLMGatewayBudgets(t.Context(), "")
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]string{}
			for _, b := range budgets {
				got[b.ID] = b.ProfileID
				if b.ID == defaultLLMGatewayBudgetID {
					continue
				}
				// A new installation gets it too, so it mustn't say that anything was carried over.
				if b.Name != "Daily token limit" || !b.Enabled || !b.PerUser || !b.ExemptAdmins || !b.UserTokenLimits ||
					strings.Contains(b.Description, "Carried over") {
					t.Errorf("unexpected budget %+v", b)
				}
				var metrics []string
				for _, l := range b.Limits {
					metrics = append(metrics, l.Metric)
					if l.Window != "1d" {
						t.Errorf("budget %s limit %s has window %s, want 1d", b.ID, l.Metric, l.Window)
					}
				}
				if !slices.Equal(metrics, tt.wantLimits) {
					t.Errorf("budget %s limits %v, want %v", b.ID, metrics, tt.wantLimits)
				}
			}
			if !maps.Equal(got, tt.want) {
				t.Fatalf("budgets %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCarryOverDailyTokenLimitsRunsOnce(t *testing.T) {
	c := newTestClient(t)
	if err := c.CarryOverDailyTokenLimits(t.Context(), staticProfiles("map-default", "map-sales"), 1000, 500); err != nil {
		t.Fatal(err)
	}
	// An administrator deletes one copy and edits the other.
	if err := c.DeleteLLMGatewayBudget(t.Context(), dailyTokenLimitBudgetID("map-sales")); err != nil {
		t.Fatal(err)
	}
	edited, err := c.GetLLMGatewayBudget(t.Context(), dailyTokenLimitBudgetID("map-default"))
	if err != nil {
		t.Fatal(err)
	}
	edited.Limits = []types.LLMGatewayBudgetLimit{{Metric: types.LLMGatewayBudgetMetricTokens, Amount: 42, Window: "1w"}}
	if err := c.UpdateLLMGatewayBudget(t.Context(), edited); err != nil {
		t.Fatal(err)
	}

	// A restart, with a profile created since: nothing comes back and the new profile gets none.
	if err := c.CarryOverDailyTokenLimits(t.Context(), staticProfiles("map-default", "map-sales", "map-new"), 1000, 500); err != nil {
		t.Fatal(err)
	}
	budgets, err := c.ListLLMGatewayBudgets(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(budgets) != 1 || budgets[0].ID != dailyTokenLimitBudgetID("map-default") ||
		len(budgets[0].Limits) != 1 || budgets[0].Limits[0].Amount != 42 {
		t.Fatalf("want only the edited copy to remain, got %+v", budgets)
	}
}

func TestEnsureDailyTokenLimitBudgetIsIdempotent(t *testing.T) {
	c := newTestClient(t)
	limits := []types.LLMGatewayBudgetLimit{{Metric: types.LLMGatewayBudgetMetricInputTokens, Amount: 1000, Window: "1d"}}
	// Replicas racing to create the same profile's copy find it instead.
	for range 2 {
		if err := c.ensureDailyTokenLimitBudget(t.Context(), "map-default", limits); err != nil {
			t.Fatal(err)
		}
	}
	budgets, err := c.ListLLMGatewayBudgets(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(budgets) != 1 {
		t.Fatalf("want one budget, got %d", len(budgets))
	}
}

func TestLLMGatewayBudgetExceededMessage(t *testing.T) {
	tests := []struct {
		name   string
		metric string
		used   float64
		amount float64
		want   string
	}{
		{
			name:   "dollars",
			metric: types.LLMGatewayBudgetMetricSpend,
			used:   3.0000000000000004,
			amount: 3,
			want:   "$3.00 of $3.00 spend used",
		},
		{
			name:   "fractions of a cent",
			metric: types.LLMGatewayBudgetMetricSpend,
			used:   0.000394,
			amount: 0.0003,
			want:   "$0.000394 of $0.0003 spend used",
		},
		{
			name:   "requests",
			metric: types.LLMGatewayBudgetMetricRequests,
			used:   2,
			amount: 2,
			want:   "2 of 2 requests used",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &LLMGatewayBudgetExceeded{
				Budget: types.LLMGatewayBudget{Name: "B", PerUser: true},
				Limit:  types.LLMGatewayBudgetLimit{Metric: tt.metric, Amount: tt.amount, Window: "1h"},
				Used:   tt.used,
			}
			if got := e.Error(); !strings.Contains(got, tt.want) {
				t.Fatalf("got %q, want it to contain %q", got, tt.want)
			}
		})
	}
}
