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

type MessagePolicyViolationHandler struct{}

func NewMessagePolicyViolationHandler() *MessagePolicyViolationHandler {
	return &MessagePolicyViolationHandler{}
}

// ListMessagePolicyViolations handles GET /api/message-policy-violations
func (*MessagePolicyViolationHandler) List(req api.Context) error {
	opts := parseMessagePolicyViolationOpts(req.URL.Query())
	if opts.Limit == 0 {
		opts.Limit = 100
	}

	violations, total, err := req.GatewayClient.GetMessagePolicyViolations(req.Context(), opts)
	if err != nil {
		return err
	}

	result := make([]types.MessagePolicyViolation, 0, len(violations))
	for _, v := range violations {
		result = append(result, types.MessagePolicyViolation{
			ID:         v.ID,
			CreatedAt:  *types.NewTime(v.CreatedAt),
			UserID:     v.UserID,
			PolicyID:   v.PolicyID,
			PolicyName: v.PolicyName,
			Direction:  v.Direction,
			ProjectID:  v.ProjectID,
			ThreadID:   v.ThreadID,
			// ViolationExplanation, PolicyDefinition, and BlockedContent
			// are excluded from list view — available in detail view only.
		})
	}

	return req.Write(types.MessagePolicyViolationResponse{
		MessagePolicyViolationList: types.MessagePolicyViolationList{Items: result},
		Total:                      total,
		Limit:                      opts.Limit,
		Offset:                     opts.Offset,
	})
}

// GetMessagePolicyViolation handles GET /api/message-policy-violations/{id}
func (*MessagePolicyViolationHandler) Get(req api.Context) error {
	idStr := req.PathValue("id")
	if idStr == "" {
		return types.NewErrBadRequest("missing policy violation id")
	}

	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return types.NewErrBadRequest("invalid policy violation id: %v", err)
	}

	v, err := req.GatewayClient.GetMessagePolicyViolation(req.Context(), uint(id))
	if err != nil {
		return err
	}

	result := types.MessagePolicyViolation{
		ID:                   v.ID,
		CreatedAt:            *types.NewTime(v.CreatedAt),
		UserID:               v.UserID,
		PolicyID:             v.PolicyID,
		PolicyName:           v.PolicyName,
		PolicyDefinition:     v.PolicyDefinition,
		Direction:            v.Direction,
		ViolationExplanation: v.ViolationExplanation,
		ProjectID:            v.ProjectID,
		ThreadID:             v.ThreadID,
	}

	// Only Auditors can see blocked content
	if req.UserIsAuditor() {
		result.BlockedContent = v.BlockedContent
	}

	return req.Write(result)
}

// ListFilterOptions handles GET /api/message-policy-violations/filter-options/{filter}
func (*MessagePolicyViolationHandler) ListFilterOptions(req api.Context) error {
	filter := req.PathValue("filter")
	if filter == "" {
		return types.NewErrBadRequest("missing filter")
	}

	validFilters := map[string]bool{
		"user_id":     true,
		"policy_id":   true,
		"policy_name": true,
		"direction":   true,
		"project_id":  true,
		"thread_id":   true,
	}
	if !validFilters[filter] {
		return types.NewErrBadRequest("invalid filter: %s", filter)
	}

	opts := parseMessagePolicyViolationOpts(req.URL.Query())
	options, err := req.GatewayClient.GetMessagePolicyViolationFilterOptions(req.Context(), filter, opts)
	if err != nil {
		return err
	}

	sort.Strings(options)

	return req.Write(map[string]any{
		"options": options,
	})
}

// GetStats handles GET /api/message-policy-violation-stats
func (*MessagePolicyViolationHandler) GetStats(req api.Context) error {
	opts := parseMessagePolicyViolationOpts(req.URL.Query())

	stats, err := req.GatewayClient.GetMessagePolicyViolationStats(req.Context(), opts)
	if err != nil {
		return err
	}

	return req.Write(types.MessagePolicyViolationStats{
		ByTime:      convertTimeBuckets(stats.ByTime),
		ByPolicy:    convertPolicyCounts(stats.ByPolicy),
		ByUser:      convertUserCounts(stats.ByUser),
		ByDirection: types.MessagePolicyViolationDirectionCounts{UserMessage: stats.ByDirection.UserMessage, ToolCalls: stats.ByDirection.ToolCalls},
	})
}

func parseMessagePolicyViolationOpts(query url.Values) gateway.MessagePolicyViolationOptions {
	opts := gateway.MessagePolicyViolationOptions{
		UserID:      parseMultiValue(query, "user_id"),
		PolicyID:    parseMultiValue(query, "policy_id"),
		Direction:   parseMultiValue(query, "direction"),
		ProjectID:   parseMultiValue(query, "project_id"),
		ThreadID:    parseMultiValue(query, "thread_id"),
		SortBy:      query.Get("sort_by"),
		SortOrder:   query.Get("sort_order"),
		Query:       strings.TrimSpace(query.Get("query")),
		TimeGroupBy: query.Get("time_group_by"),
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

func convertTimeBuckets(buckets []gateway.MessagePolicyViolationTimeBucket) []types.MessagePolicyViolationTimeBucket {
	result := make([]types.MessagePolicyViolationTimeBucket, len(buckets))
	for i, b := range buckets {
		result[i] = types.MessagePolicyViolationTimeBucket{Time: *types.NewTime(b.Time), Category: b.Category, Count: b.Count}
	}
	return result
}

func convertPolicyCounts(counts []gateway.MessagePolicyViolationPolicyCount) []types.MessagePolicyViolationPolicyCount {
	result := make([]types.MessagePolicyViolationPolicyCount, len(counts))
	for i, c := range counts {
		result[i] = types.MessagePolicyViolationPolicyCount{PolicyID: c.PolicyID, PolicyName: c.PolicyName, Count: c.Count}
	}
	return result
}

func convertUserCounts(counts []gateway.MessagePolicyViolationUserCount) []types.MessagePolicyViolationUserCount {
	result := make([]types.MessagePolicyViolationUserCount, len(counts))
	for i, c := range counts {
		result[i] = types.MessagePolicyViolationUserCount{UserID: c.UserID, Count: c.Count}
	}
	return result
}
