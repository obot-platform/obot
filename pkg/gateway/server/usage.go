package server

import (
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
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

func (s *Server) remainingUsageForUser(apiContext api.Context) error {
	userID := apiContext.PathValue("user_id")
	remainingUsage, err := apiContext.GatewayClient.RemainingTokenUsageForUser(apiContext.Context(), userID, tokenUsageTimePeriod, s.dailyUserTokenPromptTokenLimit, s.dailyUserTokenCompletionTokenLimit)
	if err != nil {
		return err
	}

	return apiContext.Write(types.ConvertRemainingTokenUsage(userID, remainingUsage))
}

func (s *Server) systemTokenUsageByUser(apiContext api.Context) error {
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

	convertedActivities := make([]types2.TokenUsage, 0, len(activities))
	for _, activity := range activities {
		convertedActivities = append(convertedActivities, types.ConvertTokenActivity(activity))
		convertedActivities[len(convertedActivities)-1].Date = types2.Time{}
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
		activity.PromptTokens += a.PromptTokens
		activity.CompletionTokens += a.CompletionTokens
		activity.TotalTokens += a.TotalTokens
	}

	return apiContext.Write(types.ConvertTokenActivity(activity))
}
