package llmengine

import (
	"bytes"
	"context"
	"runtime/pprof"
	"testing"
	"time"

	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
)

func TestEngineStopsUnusedAgentGateway(t *testing.T) {
	countRelays := func() int {
		t.Helper()

		var stacks bytes.Buffer
		if err := pprof.Lookup("goroutine").WriteTo(&stacks, 2); err != nil {
			t.Fatal(err)
		}

		return bytes.Count(stacks.Bytes(), []byte("github.com/maximhq/bifrost/core/agent.(*pushRelay).run("))
	}
	before := countRelays()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	e := New()
	if err := e.Start(ctx, testOptions(t, Options{DataDir: t.TempDir()})); err != nil {
		t.Fatal(err)
	}
	defer func() { require.NoError(t, e.Shutdown(context.Background())) }()

	if after := countRelays(); after != before {
		t.Fatalf("startup left an unused A2A push relay running: %d before, %d after", before, after)
	}

	if err := e.store.CreateProvider(ctx, "openai", llmcoreclient.Provider{}); err != nil {
		t.Fatal(err)
	}
	if changed, err := e.Sync(ctx); err != nil || !changed {
		t.Fatalf("expected configuration sync, got %v, %v", changed, err)
	}
	if after := countRelays(); after != before {
		t.Fatalf("configuration sync left an unused A2A push relay running: %d before, %d after", before, after)
	}
}
