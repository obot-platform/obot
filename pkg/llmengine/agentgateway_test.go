package llmengine

import (
	"bytes"
	"context"
	"runtime/pprof"
	"testing"
	"time"
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
	if err := e.Start(ctx, Options{DataDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	defer e.Shutdown()

	if after := countRelays(); after != before {
		t.Fatalf("startup left an unused A2A push relay running: %d before, %d after", before, after)
	}

	if reloaded, err := e.Reload(1); err != nil || !reloaded {
		t.Fatalf("expected reload, got %v, %v", reloaded, err)
	}
	if after := countRelays(); after != before {
		t.Fatalf("reload left an unused A2A push relay running: %d before, %d after", before, after)
	}
}
