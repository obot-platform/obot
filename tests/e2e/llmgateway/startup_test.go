//go:build e2e

package llmgateway

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/obot-platform/obot/tests/e2e/harness"
)

// TestFreshReplicaConfiguration installs Obot from scratch with two replicas and
// environment provider keys. Both replicas must read exactly the imported keys
// from shared resource storage, including after startup has settled.
// OBOT_E2E_STARTUP_RUNS sets the number of installations (default 5).
func TestFreshReplicaConfiguration(t *testing.T) {
	runs := 5
	if v := os.Getenv("OBOT_E2E_STARTUP_RUNS"); v != "" {
		var err error
		if runs, err = strconv.Atoi(v); err != nil {
			t.Fatalf("OBOT_E2E_STARTUP_RUNS: %v", err)
		}
	}
	binary := env.A.Config.Binary
	envKeys := map[string]string{
		"OPENAI_API_KEY":    "e2e-startup-openai-key",
		"ANTHROPIC_API_KEY": "e2e-startup-anthropic-key",
		// The gateway then logs the providers it adds and removes.
		"OBOT_SERVER_LLM_GATEWAY_LOG_LEVEL": "info",
	}
	want := []string{"anthropic/env-ANTHROPIC_API_KEY", "openai/env-OPENAI_API_KEY"}

	for run := range runs {
		t.Run(fmt.Sprintf("installation %d", run+1), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			cluster, err := harness.NewCluster(ctx, harness.ClusterOptions{
				Name: "obot-e2e-startup-" + harness.RandomSuffix(),
				Dir:  filepath.Join(env.cluster.Dir, fmt.Sprintf("startup-%d", run+1)),
			})
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := cluster.Close(); err != nil {
					t.Logf("cleanup: %v", err)
				}
			}()

			replicas := startFreshReplicas(ctx, t, cluster, binary, envKeys)
			admin, err := cluster.ActivateAdmin(ctx, replicas[0])
			if err != nil {
				t.Fatal(err)
			}

			check := func() error {
				// These APIs read persisted provider/key aggregates, not runtime caches.
				for _, r := range replicas {
					if got := replicaKeys(t, admin.On(r.BaseURL)); !slices.Equal(got, want) {
						return fmt.Errorf("%s has keys %v, want %v", r.Config.Name, got, want)
					}
				}
				return nil
			}
			harness.Eventually(t, 2*time.Minute, "the providers configured from the environment", check)
			// Nothing a replica still does after starting may remove them.
			for range 5 {
				time.Sleep(time.Second)
				if err := check(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// startFreshReplicas bootstraps Obot's schema on the first replica before starting
// the second, avoiding concurrent schema creation on a fresh database.
func startFreshReplicas(ctx context.Context, t *testing.T, cluster *harness.Cluster, binary string, env map[string]string) []*harness.Replica {
	t.Helper()
	first, err := cluster.StartReplica(ctx, "replica-a", binary, nil, env)
	if err != nil {
		t.Fatal(err)
	}
	second, err := cluster.StartReplicaNoWait("replica-b", binary, nil, env)
	if err != nil {
		t.Fatal(err)
	}

	if err := second.WaitHealthy(ctx, 3*time.Minute); err != nil {
		t.Fatal(err)
	}
	return []*harness.Replica{first, second}
}

// replicaKeys lists the gateway's keys on one replica, as provider/key names.
func replicaKeys(t *testing.T, admin *harness.Client) []string {
	t.Helper()
	var providers struct {
		Providers []struct {
			Name string `json:"name"`
		} `json:"providers"`
	}
	resp := admin.Do(t, http.MethodGet, "/api/llm-gateway/admin/providers", nil)
	if resp.Status != http.StatusOK {
		return []string{"providers: " + resp.String()}
	}
	resp.JSON(t, &providers)
	var keys []string
	for _, p := range providers.Providers {
		var list struct {
			Keys []struct {
				Name string `json:"name"`
			} `json:"keys"`
		}
		resp := admin.Do(t, http.MethodGet, "/api/llm-gateway/admin/providers/"+p.Name+"/keys", nil)
		if resp.Status != http.StatusOK {
			keys = append(keys, p.Name+": "+resp.String())
			continue
		}
		resp.JSON(t, &list)
		if len(list.Keys) == 0 {
			keys = append(keys, p.Name+"/(no keys)")
		}
		for _, k := range list.Keys {
			keys = append(keys, p.Name+"/"+k.Name)
		}
	}
	slices.Sort(keys)
	return keys
}
