package types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClaudeSubscriptionSettingJSONCompatibility(t *testing.T) {
	var settings SubscriptionPassthroughSettings
	require.NoError(t, json.Unmarshal([]byte(`{"subscriptionPassthrough":true}`), &settings))
	require.True(t, settings.ClaudeSubscriptionPassthrough)
	require.False(t, settings.ChatGPTSubscriptionPassthrough)

	settings.ClaudeSubscriptionPassthrough = false
	settings.ChatGPTSubscriptionPassthrough = true
	data, err := json.Marshal(settings)
	require.NoError(t, err)
	require.JSONEq(t, `{"subscriptionPassthrough":false,"chatgptSubscriptionPassthrough":true}`, string(data))
}
