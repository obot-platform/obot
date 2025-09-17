package server

import (
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

func (s *Server) activeUsers(apiContext api.Context) error {
	requestedStart := apiContext.Request.URL.Query().Get("start")
	requestedEnd := apiContext.Request.URL.Query().Get("end")

	start, end, err := parseDateRange(requestedStart, requestedEnd)
	if err != nil {
		return err
	}

	activeUsers, err := apiContext.GatewayClient.ActiveUsersByDate(apiContext.Context(), start, end)
	if err != nil {
		return err
	}

	items := make([]types2.User, 0, len(activeUsers))
	for _, user := range activeUsers {
		if user.Username != "bootstrap" && user.Email != "" { // Filter out the bootstrap admin
			items = append(items, *types.ConvertUser(&user, apiContext.GatewayClient.HasExplicitRole(user.Email) != types2.RoleUnknown, ""))
		}
	}

	return apiContext.Write(types2.UserList{Items: items})
}

func (s *Server) activitiesByUser(apiContext api.Context) error {
	userID := apiContext.PathValue("user_id")
	requestedStart := apiContext.Request.URL.Query().Get("start")
	requestedEnd := apiContext.Request.URL.Query().Get("end")

	start, end, err := parseDateRange(requestedStart, requestedEnd)
	if err != nil {
		return err
	}

	activities, err := apiContext.GatewayClient.ActivitiesByUser(apiContext.Context(), userID, start, end)
	if err != nil {
		return err
	}

	convertedActivities := make([]types2.APIActivity, 0, len(activities))
	for _, activity := range activities {
		convertedActivities = append(convertedActivities, types.ConvertAPIActivity(activity))
	}

	return apiContext.Write(types2.APIActivityList{Items: convertedActivities})
}

func parseDateRange(requestedStart, requestedEnd string) (time.Time, time.Time, error) {
	// Default to the last 24 hours if no date range is provided
	if requestedStart == "" {
		requestedStart = time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
	}
	if requestedEnd == "" {
		requestedEnd = time.Now().Format(time.RFC3339)
	}

	start, err := time.Parse(time.RFC3339, requestedStart)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	end, err := time.Parse(time.RFC3339, requestedEnd)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	return start, end, nil
}
