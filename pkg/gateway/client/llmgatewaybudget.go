package client

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

const (
	// defaultLLMGatewayBudgetID is the ID of the budget an earlier carry-over of the daily
	// token limits put on one profile, and the prefix of the per-profile copies' IDs.
	defaultLLMGatewayBudgetID = "daily-token-limit"
	// dailyTokenLimitBudgetsMigration records that the daily token limits were carried over to
	// every profile. It replaces llm_gateway_default_budget, which put one budget on the
	// profile for everyone; installations that ran that one run this one too, once.
	dailyTokenLimitBudgetsMigration = "llm_gateway_profile_budgets"
	// dailyTokenLimitBudgetDescription describes the daily token limit budget. It is the same
	// on a new installation as after an upgrade, so it says where the limits come from without
	// saying that anything was upgraded.
	dailyTokenLimitBudgetDescription = "Created when the LLM gateway first started, from the server's daily user token limits, for this profile's models. A user's personal daily token limits apply in its place. Edit or delete it like any other budget."
)

// LLMGatewayBudgetExceeded describes the first limit a request would exceed.
type LLMGatewayBudgetExceeded struct {
	Budget  types.LLMGatewayBudget
	Limit   types.LLMGatewayBudgetLimit
	Used    float64
	ResetAt time.Time
}

type budgetUsageTotals struct {
	InputTokens  float64
	OutputTokens float64
	TotalTokens  float64
	Spend        float64
	Requests     float64
}

// LLMGatewayBudgetUsageEntry is the usage of one user (or of everyone, for a shared
// budget) in each limit's current window, in the same order as the budget's limits.
type LLMGatewayBudgetUsageEntry struct {
	UserID string    `json:"userID,omitempty"`
	Used   []float64 `json:"used"`
}

func (e *LLMGatewayBudgetExceeded) Error() string {
	what := metricLabel(e.Limit.Metric)
	scope := "shared"
	if e.Budget.PerUser {
		scope = "your"
	}
	msg := fmt.Sprintf("%s LLM budget %q is exhausted: %s of %s %s used in the current window", scope, e.Budget.Name, formatMetric(e.Limit.Metric, e.Used), formatMetric(e.Limit.Metric, e.Limit.Amount), what)
	if !e.ResetAt.IsZero() {
		msg += fmt.Sprintf("; more becomes available by %s", e.ResetAt.UTC().Format(time.RFC3339))
	}
	return msg
}

func metricLabel(metric string) string {
	switch metric {
	case types.LLMGatewayBudgetMetricSpend:
		return "spend"
	case types.LLMGatewayBudgetMetricInputTokens:
		return "input tokens"
	case types.LLMGatewayBudgetMetricOutputTokens:
		return "output tokens"
	case types.LLMGatewayBudgetMetricRequests:
		return "requests"
	default:
		return "tokens"
	}
}

func formatMetric(metric string, v float64) string {
	if metric == types.LLMGatewayBudgetMetricSpend {
		return formatUSD(v)
	}
	return strconv.FormatInt(int64(v), 10)
}

// formatUSD formats an amount in dollars with cents, and with the fractions of a cent a small
// amount needs, up to millionths: $3.00, $1.50, $0.0004.
func formatUSD(v float64) string {
	s := strings.TrimRight(strconv.FormatFloat(v, 'f', 6, 64), "0")
	whole, cents, _ := strings.Cut(s, ".")
	for len(cents) < 2 {
		cents += "0"
	}
	return "$" + whole + "." + cents
}

// ParseLLMGatewayBudgetWindow returns the start of a limit's current window and, for
// calendar windows, when it next resets. Rolling windows report a zero reset time.
func ParseLLMGatewayBudgetWindow(limit types.LLMGatewayBudgetLimit, now time.Time) (start, reset time.Time, _ error) {
	w := strings.TrimSpace(limit.Window)
	if len(w) < 2 {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid window %q", limit.Window)
	}
	n, err := strconv.Atoi(w[:len(w)-1])
	if err != nil || n <= 0 {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid window %q", limit.Window)
	}
	unit := w[len(w)-1]
	if limit.Calendar {
		if n != 1 {
			return time.Time{}, time.Time{}, fmt.Errorf("calendar windows must be a single unit, got %q", limit.Window)
		}
		now = now.UTC()
		switch unit {
		case 'm':
			start = now.Truncate(time.Minute)
			return start, start.Add(time.Minute), nil
		case 'h':
			start = now.Truncate(time.Hour)
			return start, start.Add(time.Hour), nil
		case 'd':
			start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
			return start, start.AddDate(0, 0, 1), nil
		case 'w':
			day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
			offset := (int(day.Weekday()) + 6) % 7 // weeks start on Monday
			start = day.AddDate(0, 0, -offset)
			return start, start.AddDate(0, 0, 7), nil
		case 'M':
			start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
			return start, start.AddDate(0, 1, 0), nil
		}
		return time.Time{}, time.Time{}, fmt.Errorf("invalid window unit in %q", limit.Window)
	}
	switch unit {
	case 'm':
		start = now.Add(-time.Duration(n) * time.Minute)
	case 'h':
		start = now.Add(-time.Duration(n) * time.Hour)
	case 'd':
		start = now.AddDate(0, 0, -n)
	case 'w':
		start = now.AddDate(0, 0, -7*n)
	case 'M':
		start = now.AddDate(0, -n, 0)
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("invalid window unit in %q", limit.Window)
	}
	if start.Before(oldestBudgetWindowStart(now)) {
		return time.Time{}, time.Time{}, fmt.Errorf("window %q is longer than a year; use at most 12M, 52w or 366d", limit.Window)
	}
	return start, time.Time{}, nil
}

// oldestBudgetWindowStart is where the longest allowed rolling window starts: a year back,
// with a day's slack for a leap year's 366 days. Usage older than any window can reach is
// deleted (see deleteOldLLMGatewayBudgetUsage), so windows can't be longer.
func oldestBudgetWindowStart(now time.Time) time.Time {
	return now.AddDate(-1, 0, -1)
}

// ValidateLLMGatewayBudget checks a budget before it is saved.
func ValidateLLMGatewayBudget(b *types.LLMGatewayBudget) error {
	if strings.TrimSpace(b.Name) == "" {
		return errors.New("name is required")
	}
	if b.ProfileID == "" {
		return errors.New("profileID is required")
	}
	if len(b.Limits) == 0 {
		return errors.New("at least one limit is required")
	}
	for _, l := range b.Limits {
		switch l.Metric {
		case types.LLMGatewayBudgetMetricSpend, types.LLMGatewayBudgetMetricTokens, types.LLMGatewayBudgetMetricInputTokens,
			types.LLMGatewayBudgetMetricOutputTokens, types.LLMGatewayBudgetMetricRequests:
		default:
			return fmt.Errorf("invalid metric %q", l.Metric)
		}
		if l.Amount < 0 {
			return errors.New("limit amounts must not be negative")
		}
		if _, _, err := ParseLLMGatewayBudgetWindow(l, time.Now()); err != nil {
			return err
		}
	}
	return nil
}

// ListLLMGatewayBudgets lists the budgets of one profile, or every budget if profileID is
// empty.
func (c *Client) ListLLMGatewayBudgets(ctx context.Context, profileID string) ([]types.LLMGatewayBudget, error) {
	var budgets []types.LLMGatewayBudget
	q := c.db.WithContext(ctx).Order("created_at")
	if profileID != "" {
		q = q.Where("profile_id = ?", profileID)
	}
	return budgets, q.Find(&budgets).Error
}

func (c *Client) GetLLMGatewayBudget(ctx context.Context, id string) (*types.LLMGatewayBudget, error) {
	var budget types.LLMGatewayBudget
	if err := c.db.WithContext(ctx).Where("id = ?", id).First(&budget).Error; err != nil {
		return nil, err
	}
	return &budget, nil
}

func (c *Client) CreateLLMGatewayBudget(ctx context.Context, b *types.LLMGatewayBudget) error {
	if err := ValidateLLMGatewayBudget(b); err != nil {
		return err
	}
	b.ID = uuid.NewString()
	return c.createLLMGatewayBudget(ctx, b)
}

func (c *Client) createLLMGatewayBudget(ctx context.Context, b *types.LLMGatewayBudget) error {
	return c.db.WithContext(ctx).Create(b).Error
}

func (c *Client) UpdateLLMGatewayBudget(ctx context.Context, b *types.LLMGatewayBudget) error {
	if err := ValidateLLMGatewayBudget(b); err != nil {
		return err
	}
	return c.db.WithContext(ctx).Model(b).Select("name", "description", "enabled", "profile_id", "per_user", "exempt_admins", "user_token_limits", "limits", "models").Updates(b).Error
}

func (c *Client) DeleteLLMGatewayBudget(ctx context.Context, id string) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("budget_id = ?", id).Delete(&types.LLMGatewayBudgetUsage{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&types.LLMGatewayBudget{}).Error
	})
}

// DeleteLLMGatewayBudgetsForProfile deletes the budgets of a profile that was deleted, with
// their usage.
func (c *Client) DeleteLLMGatewayBudgetsForProfile(ctx context.Context, profileID string) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ids := tx.Model(&types.LLMGatewayBudget{}).Select("id").Where("profile_id = ?", profileID)
		if err := tx.Where("budget_id IN (?)", ids).Delete(&types.LLMGatewayBudgetUsage{}).Error; err != nil {
			return err
		}
		return tx.Where("profile_id = ?", profileID).Delete(&types.LLMGatewayBudget{}).Error
	})
}

// LLMGatewayBudgetsForUser returns the enabled budgets that apply to a user's request for a
// model. A budget covers only its own profile's models, so profileIDs must be the user's
// profiles that grant the model (see modelaccesspolicy.Helper.UserProfileIDsForModel); a
// budget of one of them applies when it covers all of its profile's models or names this
// one. Every applicable budget is enforced; a request must fit within all of them.
func (c *Client) LLMGatewayBudgetsForUser(ctx context.Context, profileIDs []string, isAdmin bool, modelID string) ([]types.LLMGatewayBudget, error) {
	if len(profileIDs) == 0 {
		return nil, nil
	}
	var budgets []types.LLMGatewayBudget
	if err := c.db.WithContext(ctx).Where("enabled = ? AND profile_id IN ?", true, profileIDs).Order("created_at").Find(&budgets).Error; err != nil {
		return nil, err
	}

	var result []types.LLMGatewayBudget
	for _, b := range budgets {
		if isAdmin && b.ExemptAdmins {
			continue
		}
		if len(b.Models) > 0 && !slices.Contains(b.Models, modelID) {
			continue
		}
		result = append(result, b)
	}
	return result, nil
}

func (t budgetUsageTotals) value(metric string) float64 {
	switch metric {
	case types.LLMGatewayBudgetMetricSpend:
		return t.Spend
	case types.LLMGatewayBudgetMetricInputTokens:
		return t.InputTokens
	case types.LLMGatewayBudgetMetricOutputTokens:
		return t.OutputTokens
	case types.LLMGatewayBudgetMetricRequests:
		return t.Requests
	default:
		return t.TotalTokens
	}
}

func (c *Client) budgetUsageSince(ctx context.Context, budgetID, userID string, since time.Time) (budgetUsageTotals, error) {
	var totals budgetUsageTotals
	q := c.db.WithContext(ctx).Model(&types.LLMGatewayBudgetUsage{}).
		Select("COALESCE(SUM(input_tokens), 0) AS input_tokens, COALESCE(SUM(output_tokens), 0) AS output_tokens, "+
			"COALESCE(SUM(total_tokens), 0) AS total_tokens, COALESCE(SUM(spend), 0) AS spend, COUNT(*) AS requests").
		Where("budget_id = ? AND created_at >= ?", budgetID, since)
	if userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	return totals, q.Scan(&totals).Error
}

// CheckLLMGatewayBudgets reports the first limit among the given budgets that the user has
// already reached. Usage is counted per user for per-user budgets and across everyone for
// shared ones.
func (c *Client) CheckLLMGatewayBudgets(ctx context.Context, budgets []types.LLMGatewayBudget, userID string) error {
	now := time.Now()
	var user *types.User
	for _, b := range budgets {
		scopeUser := ""
		if b.PerUser {
			scopeUser = userID
		}
		limits := b.Limits
		if b.PerUser && b.UserTokenLimits {
			if user == nil {
				if user, _ = c.UserByID(ctx, userID); user == nil {
					user = &types.User{}
				}
			}
			limits = withUserTokenLimits(limits, user)
		}
		for _, l := range limits {
			start, reset, err := ParseLLMGatewayBudgetWindow(l, now)
			if err != nil {
				return fmt.Errorf("budget %q has an invalid limit: %w", b.Name, err)
			}
			totals, err := c.budgetUsageSince(ctx, b.ID, scopeUser, start)
			if err != nil {
				return err
			}
			if used := totals.value(l.Metric); used >= l.Amount {
				if reset.IsZero() {
					reset, err = c.rollingWindowReset(ctx, b.ID, scopeUser, start, l)
					if err != nil {
						return err
					}
				}
				return &LLMGatewayBudgetExceeded{Budget: b, Limit: l, Used: used, ResetAt: reset}
			}
		}
	}
	return nil
}

// withUserTokenLimits applies a user's personal daily token limits to a budget's input and
// output token limits, as the daily token quota did.
func withUserTokenLimits(limits []types.LLMGatewayBudgetLimit, user *types.User) []types.LLMGatewayBudgetLimit {
	out := make([]types.LLMGatewayBudgetLimit, 0, len(limits))
	for _, l := range limits {
		personal := 0
		switch l.Metric {
		case types.LLMGatewayBudgetMetricInputTokens:
			personal = user.DailyInputTokensLimit
		case types.LLMGatewayBudgetMetricOutputTokens:
			personal = user.DailyOutputTokensLimit
		}
		switch {
		case personal < 0:
			continue
		case personal > 0:
			l.Amount = float64(personal)
		}
		out = append(out, l)
	}
	return out
}

// rollingWindowReset estimates when a rolling window frees up: when the oldest usage in it
// ages out.
func (c *Client) rollingWindowReset(ctx context.Context, budgetID, userID string, start time.Time, l types.LLMGatewayBudgetLimit) (time.Time, error) {
	var oldest types.LLMGatewayBudgetUsage
	q := c.db.WithContext(ctx).Where("budget_id = ? AND created_at >= ?", budgetID, start)
	if userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.Order("created_at").Limit(1).Find(&oldest).Error; err != nil || oldest.ID == 0 {
		return time.Time{}, err
	}
	now := time.Now()
	windowStart, _, _ := ParseLLMGatewayBudgetWindow(l, now)
	return oldest.CreatedAt.Add(now.Sub(windowStart)), nil
}

// RecordLLMGatewayBudgetUsage counts one request's usage against each of the budgets it was
// admitted under.
func (c *Client) RecordLLMGatewayBudgetUsage(ctx context.Context, budgetIDs []string, userID, model string, usage types.TokenUsage) error {
	if len(budgetIDs) == 0 {
		return nil
	}
	rows := make([]types.LLMGatewayBudgetUsage, 0, len(budgetIDs))
	now := time.Now()
	for _, id := range budgetIDs {
		rows = append(rows, types.LLMGatewayBudgetUsage{
			BudgetID:     id,
			UserID:       userID,
			CreatedAt:    now,
			Model:        model,
			InputTokens:  usage.InputTokens,
			OutputTokens: usage.OutputTokens,
			TotalTokens:  usage.TotalTokens,
			Spend:        usage.TotalSpend,
		})
	}
	return c.db.WithContext(ctx).Create(&rows).Error
}

// LLMGatewayBudgetUsageReport returns current usage for a budget: one entry per user who
// has usage in the longest window for per-user budgets, or a single total for shared ones.
func (c *Client) LLMGatewayBudgetUsageReport(ctx context.Context, b *types.LLMGatewayBudget) ([]LLMGatewayBudgetUsageEntry, error) {
	now := time.Now()
	starts := make([]time.Time, len(b.Limits))
	earliest := now
	for i, l := range b.Limits {
		start, _, err := ParseLLMGatewayBudgetWindow(l, now)
		if err != nil {
			return nil, err
		}
		starts[i] = start
		if start.Before(earliest) {
			earliest = start
		}
	}

	if !b.PerUser {
		entry := LLMGatewayBudgetUsageEntry{Used: make([]float64, len(b.Limits))}
		for i, l := range b.Limits {
			totals, err := c.budgetUsageSince(ctx, b.ID, "", starts[i])
			if err != nil {
				return nil, err
			}
			entry.Used[i] = totals.value(l.Metric)
		}
		return []LLMGatewayBudgetUsageEntry{entry}, nil
	}

	var userIDs []string
	if err := c.db.WithContext(ctx).Model(&types.LLMGatewayBudgetUsage{}).
		Where("budget_id = ? AND created_at >= ?", b.ID, earliest).
		Distinct("user_id").Pluck("user_id", &userIDs).Error; err != nil {
		return nil, err
	}
	entries := make([]LLMGatewayBudgetUsageEntry, 0, len(userIDs))
	for _, userID := range userIDs {
		entry := LLMGatewayBudgetUsageEntry{UserID: userID, Used: make([]float64, len(b.Limits))}
		for i, l := range b.Limits {
			totals, err := c.budgetUsageSince(ctx, b.ID, userID, starts[i])
			if err != nil {
				return nil, err
			}
			entry.Used[i] = totals.value(l.Metric)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// deleteOldLLMGatewayBudgetUsage removes usage older than any window can reach, with a month
// to spare. It looks before deleting: with SQLite a delete takes the write lock even when
// nothing matches, and this runs as every replica starts, alongside other writes.
func (c *Client) deleteOldLLMGatewayBudgetUsage(ctx context.Context, now time.Time) error {
	cutoff := oldestBudgetWindowStart(now).AddDate(0, -1, 0)
	var ids []uint
	if err := c.db.WithContext(ctx).Model(&types.LLMGatewayBudgetUsage{}).Where("created_at < ?", cutoff).Limit(1).Pluck("id", &ids).Error; err != nil || len(ids) == 0 {
		return err
	}
	return c.db.WithContext(ctx).Where("created_at < ?", cutoff).Delete(&types.LLMGatewayBudgetUsage{}).Error
}

// dailyTokenLimitBudgetID is the ID of the carried-over budget on a profile. It is fixed, so
// replicas starting together create one per profile.
func dailyTokenLimitBudgetID(profileID string) string {
	return defaultLLMGatewayBudgetID + "-" + profileID
}

// CarryOverDailyTokenLimits carries the server's daily token limits over to the LLM gateway,
// once: every profile that profiles returns gets an editable per-user "Daily token limit"
// budget. A budget covers only its own profile's models, so a copy on every profile keeps
// every model a user can reach covered. Profiles created later get none, and a copy an
// administrator edits or deletes stays that way. A budget from the earlier carry-over to a
// single profile (with the fixed ID defaultLLMGatewayBudgetID) counts as that profile's copy.
// Negative limits are disabled and produce no limit; with none, nothing is created and
// profiles isn't called.
func (c *Client) CarryOverDailyTokenLimits(ctx context.Context, profiles func(context.Context) ([]string, error), dailyInputTokens, dailyOutputTokens int) error {
	return c.MigrateKinmIfNotRun(ctx, dailyTokenLimitBudgetsMigration, func() error {
		var limits []types.LLMGatewayBudgetLimit
		if dailyInputTokens >= 0 {
			limits = append(limits, types.LLMGatewayBudgetLimit{Metric: types.LLMGatewayBudgetMetricInputTokens, Amount: float64(dailyInputTokens), Window: "1d"})
		}
		if dailyOutputTokens >= 0 {
			limits = append(limits, types.LLMGatewayBudgetLimit{Metric: types.LLMGatewayBudgetMetricOutputTokens, Amount: float64(dailyOutputTokens), Window: "1d"})
		}
		if len(limits) == 0 {
			return nil
		}
		profileIDs, err := profiles(ctx)
		if err != nil {
			return err
		}
		covered := map[string]bool{}
		if old, err := c.GetLLMGatewayBudget(ctx, defaultLLMGatewayBudgetID); err == nil {
			covered[old.ProfileID] = true
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		for _, profileID := range profileIDs {
			if covered[profileID] {
				continue
			}
			covered[profileID] = true
			if err := c.ensureDailyTokenLimitBudget(ctx, profileID, limits); err != nil {
				return fmt.Errorf("profile %s: %w", profileID, err)
			}
		}
		return nil
	})
}

func (c *Client) ensureDailyTokenLimitBudget(ctx context.Context, profileID string, limits []types.LLMGatewayBudgetLimit) error {
	id := dailyTokenLimitBudgetID(profileID)
	if _, err := c.GetLLMGatewayBudget(ctx, id); err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	err := c.createLLMGatewayBudget(ctx, &types.LLMGatewayBudget{
		ID:              id,
		Name:            "Daily token limit",
		Description:     dailyTokenLimitBudgetDescription,
		Enabled:         true,
		PerUser:         true,
		ExemptAdmins:    true,
		UserTokenLimits: true,
		Limits:          slices.Clone(limits),
		ProfileID:       profileID,
	})
	if err != nil {
		if _, getErr := c.GetLLMGatewayBudget(ctx, id); getErr == nil {
			// Another replica created it first.
			return nil
		}
	}
	return err
}
