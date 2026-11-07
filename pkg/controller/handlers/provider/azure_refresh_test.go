package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"github.com/obot-platform/obot/pkg/llmengine"
	"github.com/stretchr/testify/require"
)

type azureRefreshProvider struct {
	*llmengine.Engine
	ready   bool
	refresh func(context.Context) error
}

func (p azureRefreshProvider) Ready() bool { return p.ready }

func (p azureRefreshProvider) RefreshAzureDeployments(ctx context.Context) error {
	return p.refresh(ctx)
}

func TestLeaderAzureRefreshUsesRequestContextAndRetriesFailures(t *testing.T) {
	mp := llmengine.ModelProviderResource()
	dispatcher := dispatcher.New(nil, nil, nil, nil, "", "", "")
	failure := errors.New("discovery failed")
	ctx, cancel := context.WithCancel(t.Context())
	marker := new(struct{})
	ctx = context.WithValue(ctx, marker, "leader")
	var calls int
	provider := azureRefreshProvider{
		Engine: llmengine.New(),
		ready:  true,
		refresh: func(actual context.Context) error {
			calls++
			require.Equal(t, "leader", actual.Value(marker))
			if err := actual.Err(); err != nil {
				return err
			}
			return failure
		},
	}
	dispatcher.RegisterBuiltinModelProvider(mp.Namespace, mp.Name, provider)
	handler := New(nil, dispatcher, nil, nil)
	request := router.Request{Ctx: ctx, Object: mp}
	require.ErrorIs(t, handler.RefreshAzureDeployments(request, nil), failure, "the router owns retries")
	cancel()
	require.ErrorIs(t, handler.RefreshAzureDeployments(request, nil), context.Canceled)
	require.Equal(t, 2, calls)

	provider.ready = false
	dispatcher.RegisterBuiltinModelProvider(mp.Namespace, mp.Name, provider)
	require.NoError(t, handler.RefreshAzureDeployments(request, nil))
	require.Equal(t, 2, calls)
}
