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
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/obot-platform/obot/tests/e2e/harness"
)

// TestFreshReplicasStartTogether installs Obot from scratch several times, each time starting
// two replicas together on an empty database with OPENAI_API_KEY and ANTHROPIC_API_KEY set, as
// a new two-replica Kubernetes installation does. Every time, the gateway must end up with the
// openai and anthropic providers, each with only the key from its variable, in the shared
// database and on both replicas.
//
// What matters is that the second gateway loads its configuration while the first is still
// starting, and finishes starting after it. Started at the same instant, replicas rarely do
// that here: the gateway's first start creates its tables while holding a lock that another
// replica retries only every 5 seconds. So startTogether lines them up: it holds the second
// replica back just before its gateway starts, and lets it go once the first gateway has
// created its tables, a little later in each installation. OBOT_E2E_STARTUP_RUNS sets the
// number of installations (default 5).
func TestFreshReplicasStartTogether(t *testing.T) {
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

			replicas := startTogether(ctx, t, cluster, binary, envKeys, time.Duration(run%5)*150*time.Millisecond)
			admin, err := cluster.ActivateAdmin(ctx, replicas[0])
			if err != nil {
				t.Fatal(err)
			}

			gatewayDB := cluster.DB + "_llm_gateway"
			check := func() error {
				out, err := cluster.PG.Exec(ctx, gatewayDB, "select p.name || '/' || k.name from config_keys k join config_providers p on p.id = k.provider_id order by 1")
				if err != nil {
					return err
				}
				if got := strings.Fields(out); !slices.Equal(got, want) {
					return fmt.Errorf("gateway database has keys %v, want %v", got, want)
				}
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

// startTogether starts two replicas of binary on cluster so that their gateways start at the
// same time, and waits for them to serve. The first replica creates Obot's tables before the
// second starts (replicas that create them at the same moment can fail). The second is stopped
// (SIGSTOP) as its gateway begins to start, while the first gateway creates its tables, and is
// resumed delay after the first gateway has created them. The gateways log both at the info
// level.
func startTogether(ctx context.Context, t *testing.T, cluster *harness.Cluster, binary string, env map[string]string, delay time.Duration) []*harness.Replica {
	t.Helper()
	first, err := cluster.StartReplica(ctx, "replica-a", binary, nil, env)
	if err != nil {
		t.Fatal(err)
	}
	second, err := cluster.StartReplicaNoWait("replica-b", binary, nil, env)
	if err != nil {
		t.Fatal(err)
	}
	if waitForLog(t, second, "loading configuration from", time.Minute) && !strings.Contains(first.Log(), "schema migrations complete") {
		_ = syscall.Kill(-second.PID(), syscall.SIGSTOP)
		waitForLog(t, first, "schema migrations complete", 2*time.Minute)
		time.Sleep(delay)
		_ = syscall.Kill(-second.PID(), syscall.SIGCONT)
	} else {
		t.Logf("the first gateway created its tables before the second began to start")
	}
	if err := second.WaitHealthy(ctx, 3*time.Minute); err != nil {
		t.Fatal(err)
	}
	return []*harness.Replica{first, second}
}

// waitForLog waits for text to appear in r's log, and reports whether it did.
func waitForLog(t *testing.T, r *harness.Replica, text string, timeout time.Duration) bool {
	t.Helper()
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if strings.Contains(r.Log(), text) {
			return true
		}
		if r.Exited() {
			t.Fatalf("%s exited; log tail:\n%s", r.Config.Name, r.LogTail(40))
		}
	}
	t.Logf("%s did not log %q within %s", r.Config.Name, text, timeout)
	return false
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
