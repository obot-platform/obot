package handlers

import (
	"context"
	"fmt"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
)

type MockDataHandler struct {
	userLimitProvider client.UserLimitProvider
}

func NewMockDataHandler(userLimitProvider client.UserLimitProvider) *MockDataHandler {
	return &MockDataHandler{
		userLimitProvider: userLimitProvider,
	}
}

func (h *MockDataHandler) Generate(req api.Context) error {
	userLimit, err := resolveMockDataUserLimit(req.Context(), h.userLimitProvider)
	if err != nil {
		return err
	}

	result, err := req.GatewayClient.GenerateMockData(req.Context(), userLimit)
	if err != nil {
		return err
	}
	return req.WriteCreated(result)
}

func resolveMockDataUserLimit(ctx context.Context, provider client.UserLimitProvider) (client.UserLimit, error) {
	userLimit, err := provider.UserLimit(ctx)
	if err != nil {
		return client.UserLimit{}, fmt.Errorf("failed to resolve user limit: %w", err)
	}
	if !userLimit.Unlimited && userLimit.Maximum <= 0 {
		return client.UserLimit{}, fmt.Errorf("invalid user limit %d", userLimit.Maximum)
	}
	return userLimit, nil
}
