package handlers

import (
	"github.com/obot-platform/obot/pkg/api"
)

type MockDataHandler struct{}

func NewMockDataHandler() *MockDataHandler {
	return nil
}

func (*MockDataHandler) Generate(req api.Context) error {
	result, err := req.GatewayClient.GenerateMockData(req.Context())
	if err != nil {
		return err
	}
	return req.WriteCreated(result)
}
