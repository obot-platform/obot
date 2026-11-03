package llmengine

import (
	"context"
	"testing"

	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type plaintextConfigCodec struct{}

func (plaintextConfigCodec) SealGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	return data, nil
}
func (plaintextConfigCodec) OpenGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	return data, nil
}
func testOptions(t *testing.T, opts Options) Options {
	t.Helper()
	if opts.Store != nil {
		return opts
	}
	var err error
	opts.Store, err = llmcoreclient.New(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(), plaintextConfigCodec{})
	if err != nil {
		t.Fatal(err)
	}
	return opts
}
