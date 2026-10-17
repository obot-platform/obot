package server

import (
	"strings"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

func (s *Server) usageForUser(apiContext api.Context) error {
	userID := apiContext.PathValue("user_id")
	requestedStart := apiContext.Request.URL.Query().Get("start")
	requestedEnd := apiContext.Request.URL.Query().Get("end")

	start, end, err := parseDateRange(requestedStart, requestedEnd)
	if err != nil {
		return err
	}

	activities, err := apiContext.GatewayClient.TokenUsageForUser(apiContext.Context(), userID, start, end)
	if err != nil {
		return err
	}

	convertedActivities := make([]types2.TokenUsage, 0, len(activities))
	for _, activity := range activities {
		convertedActivities = append(convertedActivities, types.ConvertTokenActivity(activity))
	}

	return apiContext.Write(types2.TokenUsageList{Items: convertedActivities})
}

func (s *Server) totalUsageForUser(apiContext api.Context) error {
	userID := apiContext.PathValue("user_id")
	requestedStart := apiContext.Request.URL.Query().Get("start")
	requestedEnd := apiContext.Request.URL.Query().Get("end")

	start, end, err := parseDateRange(requestedStart, requestedEnd)
	if err != nil {
		return err
	}

	activity, err := apiContext.GatewayClient.TotalTokenUsageForUser(apiContext.Context(), userID, start, end)
	if err != nil {
		return err
	}

	// Clear the created at time since it is not relevant for this endpoint.
	activity.CreatedAt = time.Time{}

	return apiContext.Write(types.ConvertTokenActivity(activity))
}

func (s *Server) systemTokenUsageByUser(apiContext api.Context) error {
	requestedStart := apiContext.Request.URL.Query().Get("start")
	requestedEnd := apiContext.Request.URL.Query().Get("end")

	start, end, err := parseDateRange(requestedStart, requestedEnd)
	if err != nil {
		return err
	}

	activities, err := apiContext.GatewayClient.TokenUsageSeriesInRange(apiContext.Context(), start, end)
	if err != nil {
		return err
	}

	convertedActivities := make([]types2.TokenUsage, 0, len(activities))
	for _, a := range activities {
		convertedActivities = append(convertedActivities, types.ConvertTokenActivity(a))
	}

	return apiContext.Write(types2.TokenUsageList{Items: convertedActivities})
}

func (s *Server) totalSystemTokenUsage(apiContext api.Context) error {
	requestedStart := apiContext.Request.URL.Query().Get("start")
	requestedEnd := apiContext.Request.URL.Query().Get("end")

	start, end, err := parseDateRange(requestedStart, requestedEnd)
	if err != nil {
		return err
	}

	activities, err := apiContext.GatewayClient.TokenUsageByUser(apiContext.Context(), start, end)
	if err != nil {
		return err
	}

	var activity types.RunTokenActivity
	for _, a := range activities {
		activity.Usage.InputTokens += a.Usage.InputTokens
		activity.Usage.CacheReadTokens += a.Usage.CacheReadTokens
		activity.Usage.CacheWriteTokens += a.Usage.CacheWriteTokens
		activity.Usage.OutputTokens += a.Usage.OutputTokens
		activity.Usage.ThinkingTokens += a.Usage.ThinkingTokens
		activity.Usage.TotalTokens += a.Usage.TotalTokens
		activity.Usage.InputSpend += a.Usage.InputSpend
		activity.Usage.CacheReadSpend += a.Usage.CacheReadSpend
		activity.Usage.CacheWriteSpend += a.Usage.CacheWriteSpend
		activity.Usage.OutputSpend += a.Usage.OutputSpend
		activity.Usage.TotalSpend += a.Usage.TotalSpend
	}

	return apiContext.Write(types.ConvertTokenActivity(activity))
}

// tokenUsageStats summarizes LLM gateway requests over a time range: totals, a time histogram,
// spend by provider and the top models. The users, models and API keys it covers can be
// narrowed with user_id, model and api_key_id, each repeatable or comma-separated.
func (s *Server) tokenUsageStats(apiContext api.Context) error {
	query := apiContext.URL.Query()
	start, end, err := parseDateRange(query.Get("start"), query.Get("end"))
	if err != nil {
		return types2.NewErrBadRequest("invalid time range: %v", err)
	}
	if !end.After(start) {
		return types2.NewErrBadRequest("the end of the time range must be after its start")
	}

	stats, err := apiContext.GatewayClient.TokenUsageStats(apiContext.Context(), client.TokenUsageStatsOptions{
		Start:     start,
		End:       end,
		UserIDs:   splitQueryList(query["user_id"]),
		Models:    splitQueryList(query["model"]),
		APIKeyIDs: api.ParseUintList(splitQueryList(query["api_key_id"])),
	})
	if err != nil {
		return err
	}
	return apiContext.Write(stats)
}

// splitQueryList returns the values of a repeatable, comma-separated query parameter.
func splitQueryList(values []string) []string {
	var result []string
	for _, value := range values {
		for part := range strings.SplitSeq(value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				result = append(result, part)
			}
		}
	}
	return result
}
