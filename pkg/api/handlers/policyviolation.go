package handlers

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	types "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
)

type PolicyViolationHandler struct{}

func NewPolicyViolationHandler() *PolicyViolationHandler {
	return &PolicyViolationHandler{}
}

// ListPolicyViolations handles GET /api/policy-violations
func (*PolicyViolationHandler) List(req api.Context) error {
	opts := parsePolicyViolationOpts(req.URL.Query())
	if opts.Limit == 0 {
		opts.Limit = 100
	}

	violations, total, err := req.GatewayClient.GetPolicyViolations(req.Context(), opts)
	if err != nil {
		return err
	}

	result := make([]types.PolicyViolation, 0, len(violations))
	for _, v := range violations {
		result = append(result, types.PolicyViolation{
			ID:        v.ID,
			CreatedAt: *types.NewTime(v.CreatedAt),
			UserID:    v.UserID,
			PolicyID:  v.PolicyID,
			PolicyName: v.PolicyName,
			Direction: v.Direction,
			ProjectID: v.ProjectID,
			ThreadID:  v.ThreadID,
			// ViolationExplanation, PolicyDefinition, and BlockedContent
			// are excluded from list view — available in detail view only.
		})
	}

	return req.Write(types.PolicyViolationResponse{
		PolicyViolationList: types.PolicyViolationList{Items: result},
		Total:               total,
		Limit:               opts.Limit,
		Offset:              opts.Offset,
	})
}

// GetPolicyViolation handles GET /api/policy-violations/{id}
func (*PolicyViolationHandler) Get(req api.Context) error {
	idStr := req.PathValue("id")
	if idStr == "" {
		return types.NewErrBadRequest("missing policy violation id")
	}

	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return types.NewErrBadRequest("invalid policy violation id: %v", err)
	}

	v, err := req.GatewayClient.GetPolicyViolation(req.Context(), uint(id))
	if err != nil {
		return err
	}

	return req.Write(types.PolicyViolation{
		ID:                   v.ID,
		CreatedAt:            *types.NewTime(v.CreatedAt),
		UserID:               v.UserID,
		PolicyID:             v.PolicyID,
		PolicyName:           v.PolicyName,
		PolicyDefinition:     v.PolicyDefinition,
		Direction:            v.Direction,
		ViolationExplanation: v.ViolationExplanation,
		BlockedContent:       v.BlockedContent,
		ProjectID:            v.ProjectID,
		ThreadID:             v.ThreadID,
	})
}

// ListFilterOptions handles GET /api/policy-violations/filter-options/{filter}
func (*PolicyViolationHandler) ListFilterOptions(req api.Context) error {
	filter := req.PathValue("filter")
	if filter == "" {
		return types.NewErrBadRequest("missing filter")
	}

	validFilters := map[string]bool{
		"user_id":    true,
		"policy_id":  true,
		"policy_name": true,
		"direction":  true,
		"project_id": true,
		"thread_id":  true,
	}
	if !validFilters[filter] {
		return types.NewErrBadRequest("invalid filter: %s", filter)
	}

	opts := parsePolicyViolationOpts(req.URL.Query())
	options, err := req.GatewayClient.GetPolicyViolationFilterOptions(req.Context(), filter, opts)
	if err != nil {
		return err
	}

	sort.Strings(options)

	return req.Write(map[string]any{
		"options": options,
	})
}

// GetStats handles GET /api/policy-violation-stats
func (*PolicyViolationHandler) GetStats(req api.Context) error {
	opts := parsePolicyViolationOpts(req.URL.Query())

	stats, err := req.GatewayClient.GetPolicyViolationStats(req.Context(), opts)
	if err != nil {
		return err
	}

	return req.Write(types.PolicyViolationStats{
		ByTime:      convertTimeBuckets(stats.ByTime),
		ByPolicy:    convertPolicyCounts(stats.ByPolicy),
		ByUser:      convertUserCounts(stats.ByUser),
		ByDirection: types.PolicyViolationDirectionCounts{UserMessage: stats.ByDirection.UserMessage, ToolCalls: stats.ByDirection.ToolCalls},
	})
}

func parsePolicyViolationOpts(query url.Values) gateway.PolicyViolationOptions {
	opts := gateway.PolicyViolationOptions{
		UserID:    parseMultiValue(query, "user_id"),
		PolicyID:  parseMultiValue(query, "policy_id"),
		Direction: parseMultiValue(query, "direction"),
		ProjectID: parseMultiValue(query, "project_id"),
		ThreadID:  parseMultiValue(query, "thread_id"),
		SortBy:    query.Get("sort_by"),
		SortOrder: query.Get("sort_order"),
		Query:     strings.TrimSpace(query.Get("query")),
	}

	if startTime := query.Get("start_time"); startTime != "" {
		if t, err := time.Parse(time.RFC3339, startTime); err == nil {
			opts.StartTime = t
		}
	}
	if endTime := query.Get("end_time"); endTime != "" {
		if t, err := time.Parse(time.RFC3339, endTime); err == nil {
			opts.EndTime = t
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

func parseMultiValue(queryValues url.Values, key string) []string {
	values := queryValues[key]
	if len(values) == 0 {
		return nil
	}

	var result []string
	for _, value := range values {
		if value == "" {
			continue
		}
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				result = append(result, part)
			}
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func convertTimeBuckets(buckets []gateway.PolicyViolationTimeBucket) []types.PolicyViolationTimeBucket {
	result := make([]types.PolicyViolationTimeBucket, len(buckets))
	for i, b := range buckets {
		result[i] = types.PolicyViolationTimeBucket{Time: b.Time, Count: b.Count}
	}
	return result
}

func convertPolicyCounts(counts []gateway.PolicyViolationPolicyCount) []types.PolicyViolationPolicyCount {
	result := make([]types.PolicyViolationPolicyCount, len(counts))
	for i, c := range counts {
		result[i] = types.PolicyViolationPolicyCount{PolicyID: c.PolicyID, PolicyName: c.PolicyName, Count: c.Count}
	}
	return result
}

func convertUserCounts(counts []gateway.PolicyViolationUserCount) []types.PolicyViolationUserCount {
	result := make([]types.PolicyViolationUserCount, len(counts))
	for i, c := range counts {
		result[i] = types.PolicyViolationUserCount{UserID: c.UserID, Count: c.Count}
	}
	return result
}

