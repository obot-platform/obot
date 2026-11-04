package client_test

import (
	"context"
	"testing"

	"github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// plaintextTestCodec explicitly bypasses envelopes for storage-only tests.
// Production callers must supply the gateway credential codec.
type plaintextTestCodec struct{}

func (plaintextTestCodec) SealGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	return data, nil
}
func (plaintextTestCodec) OpenGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	return data, nil
}

func newStore(t *testing.T, storage kclient.Client) *client.Client {
	t.Helper()
	store, err := client.New(storage, plaintextTestCodec{})
	require.NoError(t, err)
	return store
}

func TestNewRequiresCodec(t *testing.T) {
	store, err := client.New(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(), nil)
	require.ErrorContains(t, err, "codec is required")
	require.Nil(t, store)
}
