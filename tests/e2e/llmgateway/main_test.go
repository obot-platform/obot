//go:build e2e

// Package llmgateway is an end-to-end suite for Obot's LLM gateway, budgets, guardrails and
// guardrail filters. It runs two real Obot replicas (separate processes of the obot binary)
// against one Postgres database, with an in-process mock LLM upstream and MCP server.
//
// Run it with `make test-e2e-llm-gateway`. It needs docker and a built obot binary
// (bin/obot, or OBOT_E2E_BINARY).
package llmgateway

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/tests/e2e/harness"
)

const (
	openAIKey    = "e2e-upstream-openai-key"
	anthropicKey = "e2e-upstream-anthropic-key"
)

var (
	// openAIModels and anthropicModels are what the mock lists for each provider. Models
	// named "broken" always fail. The vip model is granted to one user only, and the secret
	// model to no one.
	openAIModels    = []string{"gpt-e2e-a", "gpt-e2e-b", "gpt-e2e-broken", "gpt-e2e-vip", "gpt-e2e-secret", "text-embedding-e2e"}
	anthropicModels = []string{"claude-e2e-a", "claude-e2e-b", "claude-e2e-broken"}

	env *testEnv
)

type testEnv struct {
	cluster *harness.Cluster
	// A and B are two replicas sharing one database.
	A, B *harness.Replica
	mock *harness.MockLLM
	mcp  *harness.MockMCP

	// admin is the owner's browser session on replica A.
	admin *harness.Client
	// adminUser is the owner, with an LLM gateway API key.
	adminUser *testUser
	// vipUser is the only user granted the vip model.
	vipUser *testUser

	mu sync.Mutex
	// models maps provider/model to Obot model IDs.
	models map[string]string
}

type testUser struct {
	Email    string
	Password string
	ID       string
	Session  *harness.Client
	// Key is an API key with LLM proxy and MCP access.
	Key string
}

// setupT adapts setup failures to errors: it is a testing.TB whose Fatalf panics with an
// error that setup recovers.
type setupT struct {
	testing.TB
}

type setupFailure struct{ msg string }

// Gateway returns a client for the user's API key on replica r.
func (u *testUser) Gateway(r *harness.Replica) *harness.Client {
	return harness.NewAPIKeyClient(r.BaseURL, u.Key)
}

func TestMain(m *testing.M) {
	code, err := run(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

func run(m *testing.M) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	binary := os.Getenv("OBOT_E2E_BINARY")
	if binary == "" {
		binary = filepath.Join(repoRoot(), "bin", "obot")
	}
	dir := os.Getenv("OBOT_E2E_DIR")
	if dir == "" {
		var err error
		if dir, err = os.MkdirTemp("", "obot-e2e-llmgateway-"); err != nil {
			return 1, err
		}
	}
	fmt.Fprintf(os.Stderr, "e2e: replica logs and data in %s\n", dir)

	replicaBinary, err := harness.PrepareBinary(binary, filepath.Join(dir, "bin"))
	if err != nil {
		return 1, err
	}

	e := &testEnv{models: map[string]string{}}
	if e.mock, err = harness.StartMockLLM(); err != nil {
		return 1, err
	}
	defer e.mock.Close()
	if e.mcp, err = harness.StartMockMCP(); err != nil {
		return 1, err
	}
	defer e.mcp.Close()
	e.mock.SetModels(openAIKey, openAIModels...)
	e.mock.SetModels(anthropicKey, anthropicModels...)

	e.cluster, err = harness.NewCluster(ctx, harness.ClusterOptions{
		Name: "obot-e2e-" + harness.RandomSuffix(),
		Dir:  dir,
	})
	if err != nil {
		return 1, err
	}
	code := 1
	defer func() {
		// OBOT_E2E_KEEP=always leaves the replicas and containers running; =onfail does so
		// only when the run fails. Remove them with `docker rm -f` on the obot-e2e label
		// and by stopping the obot-e2e-replica processes.
		if keep := os.Getenv("OBOT_E2E_KEEP"); keep == "always" || keep == "onfail" && code != 0 {
			fmt.Fprintf(os.Stderr, "e2e: OBOT_E2E_KEEP=%s; leaving the cluster running in %s\n", keep, dir)
			return
		}
		if err := e.cluster.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: cleanup: %v\n", err)
		}
		// Logs and data stay for inspection; the binary copy is large.
		_ = os.RemoveAll(filepath.Join(dir, "bin"))
	}()

	// Replica A creates the schema first: two Obot processes initializing an empty
	// database at the same moment race on CREATE TABLE in the storage layer (see the
	// report). B then starts while A's gateway is still starting. Neither replica has
	// --llm-gateway-encryption-key, so they must share the generated key through the
	// database.
	if e.A, err = e.cluster.StartReplica(ctx, "replica-a", replicaBinary, nil, nil); err != nil {
		return 1, err
	}
	if e.B, err = e.cluster.StartReplica(ctx, "replica-b", replicaBinary, nil, nil); err != nil {
		return 1, err
	}

	if err := e.setup(ctx); err != nil {
		return 1, err
	}
	env = e

	code = m.Run()

	for _, r := range []*harness.Replica{e.A, e.B} {
		if lines := harness.PanicLines(r.Log()); len(lines) > 0 {
			fmt.Fprintf(os.Stderr, "e2e: %s logged a crash:\n%s\n", r.Config.Name, strings.Join(lines, "\n"))
			code = 1
		}
		if r.Exited() {
			fmt.Fprintf(os.Stderr, "e2e: %s exited during the run; log %s\n", r.Config.Name, r.LogPath)
			code = 1
		}
	}
	return code, nil
}

func (setupT) Helper() {}
func (setupT) Fatalf(format string, args ...any) {
	panic(setupFailure{msg: fmt.Sprintf(format, args...)})
}
func (s setupT) Fatal(args ...any) { s.Fatalf("%s", fmt.Sprint(args...)) }
func (setupT) Logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "e2e: "+format+"\n", args...)
}
func (setupT) Cleanup(func()) {}

func (e *testEnv) setup(ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			f, ok := r.(setupFailure)
			if !ok {
				panic(r)
			}
			err = fmt.Errorf("setup: %s", f.msg)
		}
	}()
	t := setupT{}

	if e.admin, err = e.cluster.ActivateAdmin(ctx, e.A); err != nil {
		return err
	}
	var me types.User
	e.admin.Get(t, "/api/me", &me)
	e.adminUser = &testUser{Email: harness.AdminEmail, Password: harness.AdminPassword, ID: me.ID, Session: e.admin}
	e.adminUser.Key = createAPIKey(t, e.admin, "e2e-admin")

	// The gateway starts in the background on each replica.
	for _, r := range []*harness.Replica{e.A, e.B} {
		admin := e.admin.On(r.BaseURL)
		harness.Eventually(t, 2*time.Minute, "gateway ready on "+r.Config.Name, func() error {
			var status struct {
				Ready bool   `json:"ready"`
				Error string `json:"error"`
			}
			resp := admin.Do(t, http.MethodGet, "/api/llm-gateway/status", nil)
			if resp.Status != http.StatusOK {
				return fmt.Errorf("status: %s", resp)
			}
			resp.JSON(t, &status)
			if !status.Ready {
				return fmt.Errorf("not ready: %s", status.Error)
			}
			return nil
		})
	}

	addProvider(t, e.admin, "openai", e.mock.URL, openAIKey, "")
	addProvider(t, e.admin, "anthropic", e.mock.URL, anthropicKey, "")

	var want []string
	for _, m := range openAIModels {
		want = append(want, "openai/"+m)
	}
	for _, m := range anthropicModels {
		want = append(want, "anthropic/"+m)
	}
	e.waitForModels(t, e.admin, want...)

	// Every user may use every model except the vip and secret ones; only vipUser gets the
	// vip model.
	var everyone []types.ModelResource
	for _, target := range want {
		// Model access policies only grant LLM models, so the embedding model is left out.
		if strings.Contains(target, "vip") || strings.Contains(target, "secret") || strings.Contains(target, "embed") {
			continue
		}
		everyone = append(everyone, types.ModelResource{ID: e.models[target]})
	}
	e.admin.Post(t, "/api/model-access-policies", types.ModelAccessPolicyManifest{
		DisplayName: "e2e everyone",
		Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
		Models:      everyone,
	}, nil)

	e.vipUser = e.newUser(t, "vip")
	e.admin.Post(t, "/api/model-access-policies", types.ModelAccessPolicyManifest{
		DisplayName: "e2e vip",
		Subjects:    []types.Subject{{Type: types.SubjectTypeUser, ID: e.vipUser.ID}},
		Models:      []types.ModelResource{{ID: e.models["openai/gpt-e2e-vip"]}},
	}, nil)

	// Policies reach each replica through its informers.
	for _, r := range []*harness.Replica{e.A, e.B} {
		e.waitForModelAccess(t, e.vipUser.Gateway(r), "openai/gpt-e2e-vip")
		e.waitForModelAccess(t, e.adminUser.Gateway(r), "anthropic/claude-e2e-a")
	}
	return nil
}

// createAPIKey creates an API key for the session's user with LLM proxy and MCP access.
func createAPIKey(tb testing.TB, session *harness.Client, name string) string {
	tb.Helper()
	var key struct {
		Key string `json:"key"`
	}
	session.Post(tb, "/api/api-keys", map[string]any{
		"name":              name,
		"canAccessLLMProxy": true,
		"canAccessAPI":      true,
		"mcpServerIds":      []string{"*"},
	}, &key)
	if key.Key == "" {
		tb.Fatalf("API key %s was created without a key", name)
	}
	return key.Key
}

// newUser creates a local user, signs them in and gives them an API key. Every user is
// granted the non-vip models by the "everyone" policy.
func (e *testEnv) newUser(tb testing.TB, prefix string) *testUser {
	tb.Helper()
	u := &testUser{
		Email:    fmt.Sprintf("%s-%s@e2e.example.com", prefix, harness.RandomSuffix()),
		Password: "E2e-User-Passw0rd!" + harness.RandomSuffix(),
	}
	e.admin.Post(tb, "/api/local-auth/users", map[string]any{
		"email":                 u.Email,
		"password":              u.Password,
		"requirePasswordChange": false,
	}, nil)
	session, err := harness.Login(context.Background(), e.A.BaseURL, u.Email, u.Password)
	if err != nil {
		tb.Fatalf("login %s: %v", u.Email, err)
	}
	u.Session = session
	var me types.User
	session.Get(tb, "/api/me", &me)
	u.ID = me.ID
	u.Key = createAPIKey(tb, session, prefix)
	return u
}

// addProvider configures a gateway provider pointing at the mock through the admin
// pass-through. customBase makes it a custom provider of that base type.
func addProvider(tb testing.TB, admin *harness.Client, name, baseURL, key, customBase string) {
	tb.Helper()
	provider := map[string]any{
		"provider": name,
		"network_config": map[string]any{
			"base_url":                           baseURL,
			"allow_private_network":              true,
			"default_request_timeout_in_seconds": 30,
		},
		"concurrency_and_buffer_size": map[string]any{"concurrency": 50, "buffer_size": 500},
	}
	if customBase != "" {
		provider["custom_provider_config"] = map[string]any{"base_provider_type": customBase}
	}
	admin.Post(tb, "/api/llm-gateway/admin/providers", provider, nil)
	addKey(tb, admin, name, "key-"+harness.RandomSuffix(), key)
}

// addKey adds an upstream key to a gateway provider and returns its ID.
func addKey(tb testing.TB, admin *harness.Client, provider, name, value string) string {
	tb.Helper()
	var created map[string]any
	admin.Post(tb, "/api/llm-gateway/admin/providers/"+provider+"/keys", map[string]any{
		"name":   name,
		"value":  value,
		"models": []string{"*"},
		"weight": 1,
	}, &created)
	return findKeyID(tb, admin, provider, name)
}

// findKeyID returns the ID of the named key, or "" when there is none.
func findKeyID(tb testing.TB, admin *harness.Client, provider, name string) string {
	tb.Helper()
	var keys struct {
		Keys []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"keys"`
	}
	admin.Get(tb, "/api/llm-gateway/admin/providers/"+provider+"/keys", &keys)
	for _, k := range keys.Keys {
		if k.Name == name {
			return k.ID
		}
	}
	return ""
}

// waitForModels syncs the gateway's models and waits until Obot has a Model for each
// target (provider/model), recording their IDs.
func (e *testEnv) waitForModels(tb testing.TB, admin *harness.Client, targets ...string) {
	tb.Helper()
	admin.Post(tb, "/api/llm-gateway/sync-models", map[string]any{}, nil)
	harness.Eventually(tb, 2*time.Minute, "gateway models "+strings.Join(targets, ", "), func() error {
		var list types.ModelList
		admin.Get(tb, "/api/models?all=true", &list)
		found := map[string]string{}
		for _, m := range list.Items {
			if m.ModelProvider == system.LLMGatewayModelProvider && m.Active {
				found[m.TargetModel] = m.ID
			}
		}
		var missing []string
		for _, target := range targets {
			if found[target] == "" {
				missing = append(missing, target)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("missing %v", missing)
		}
		e.mu.Lock()
		for _, target := range targets {
			e.models[target] = found[target]
		}
		e.mu.Unlock()
		return nil
	})
}

// modelID returns the Obot model ID of a gateway model.
func (e *testEnv) modelID(tb testing.TB, target string) string {
	tb.Helper()
	e.mu.Lock()
	defer e.mu.Unlock()
	id := e.models[target]
	if id == "" {
		tb.Fatalf("no model ID for %s", target)
	}
	return id
}

// listedModels returns the model IDs /v1/models lists for a gateway client.
func listedModels(tb testing.TB, c *harness.Client) []string {
	tb.Helper()
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	c.Get(tb, "/api/llm-proxy/v1/models", &list)
	ids := make([]string, 0, len(list.Data))
	for _, m := range list.Data {
		ids = append(ids, m.ID)
	}
	return ids
}

// waitForModelAccess waits until the client's /v1/models lists the model.
func (e *testEnv) waitForModelAccess(tb testing.TB, c *harness.Client, target string) {
	tb.Helper()
	harness.Eventually(tb, time.Minute, target+" listed for "+c.BaseURL, func() error {
		if ids := listedModels(tb, c); !slices.Contains(ids, target) {
			return fmt.Errorf("listed %v", ids)
		}
		return nil
	})
}
