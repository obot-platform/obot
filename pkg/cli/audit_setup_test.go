package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditSetupAllCreatesExpectedHookFilesAndIsIdempotent(t *testing.T) {
	home := t.TempDir()
	binary := filepath.Join(home, "bin", "obot")

	for _, client := range auditSupportedClients() {
		result := installAuditHook(t.Context(), home, binary, client)
		if result.Error != "" || result.Malformed || !result.Installed {
			t.Fatalf("install %s = %+v", client, result)
		}
	}
	for _, client := range auditSupportedClients() {
		result := installAuditHook(t.Context(), home, binary, client)
		if result.Error != "" || result.Malformed || !result.Installed {
			t.Fatalf("second install %s = %+v", client, result)
		}
	}

	assertManagedClaudeLikeHookCount(t, filepath.Join(home, ".claude", "settings.json"), "PostToolUse", 1)
	assertManagedClaudeLikeHookCount(t, filepath.Join(home, ".claude", "settings.json"), "PostToolUseFailure", 1)
	assertManagedClaudeLikeHookCount(t, filepath.Join(home, ".codex", "hooks.json"), "PostToolUse", 1)
	assertManagedFlatHookCount(t, filepath.Join(home, ".cursor", "hooks.json"), "postToolUse", 1)
	assertManagedFlatHookCount(t, filepath.Join(home, ".cursor", "hooks.json"), "postToolUseFailure", 1)
	assertManagedFlatHookCount(t, filepath.Join(home, ".copilot", "hooks", "obot-audit.json"), "PostToolUse", 1)
}

func TestAuditSetupPreservesUnrelatedHooks(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".cursor", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	original := `{
  "version": 1,
  "hooks": {
    "postToolUse": [
      {
        "command": "./user-hook.sh",
        "env": {
          "USER_HOOK": "1"
        }
      }
    ]
  }
}`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}

	result := installAuditHook(t.Context(), home, "/usr/local/bin/obot", auditClientCursor)
	if result.Error != "" || result.Malformed || !result.Installed {
		t.Fatalf("install = %+v", result)
	}

	var root map[string]any
	readJSONFile(t, path, &root)
	hooks := root["hooks"].(map[string]any)
	post := hooks["postToolUse"].([]any)
	if len(post) != 2 {
		t.Fatalf("postToolUse hooks = %d, want 2", len(post))
	}
	if !strings.Contains(mustMarshalString(t, post), "./user-hook.sh") {
		t.Fatalf("unrelated hook was not preserved: %s", mustMarshalString(t, post))
	}
}

func TestAuditSetupSkipsMalformedConfig(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"hooks":`), 0600); err != nil {
		t.Fatal(err)
	}

	result := installAuditHook(t.Context(), home, "/usr/local/bin/obot", auditClientClaudeCode)
	if !result.Malformed || result.Installed || result.Error == "" {
		t.Fatalf("install = %+v, want malformed skip", result)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"hooks":` {
		t.Fatalf("malformed config was overwritten: %q", data)
	}
}

func TestAuditSetupSkipsMalformedHookSchema(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".cursor", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"hooks":{"postToolUse":{"command":"bad"}}}`), 0600); err != nil {
		t.Fatal(err)
	}

	result := installAuditHook(t.Context(), home, "/usr/local/bin/obot", auditClientCursor)
	if !result.Malformed || result.Installed || result.Error == "" {
		t.Fatalf("install = %+v, want malformed skip", result)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"hooks":{"postToolUse":{"command":"bad"}}}` {
		t.Fatalf("malformed hook schema was overwritten: %q", data)
	}
}

func TestAuditSetupClientSelectionAllDetectedAndExplicit(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0700); err != nil {
		t.Fatal(err)
	}

	oldLookPath := lookPath
	lookPath = func(string) (string, error) {
		return "", os.ErrNotExist
	}
	defer func() {
		lookPath = oldLookPath
	}()

	detected, err := parseAuditSetupClients(t.Context(), home, "detected")
	if err != nil {
		t.Fatal(err)
	}
	if len(detected) != 1 || detected[0] != auditClientCodex {
		t.Fatalf("detected clients = %v, want [codex]", detected)
	}

	all, err := parseAuditSetupClients(t.Context(), home, "all")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != len(auditSupportedClients()) {
		t.Fatalf("all clients = %v", all)
	}

	explicit, err := parseAuditSetupClients(t.Context(), home, "vscode")
	if err != nil {
		t.Fatal(err)
	}
	if len(explicit) != 1 || explicit[0] != auditClientVSCode {
		t.Fatalf("explicit clients = %v, want [vscode]", explicit)
	}
}

func TestAuditSetupCodexInlineConfigUpdatedWhenPresent(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[hooks]\n\n[[hooks.PreToolUse]]\nmatcher = \"^Bash$\"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		result := installAuditHook(t.Context(), home, "/usr/local/bin/obot", auditClientCodex)
		if result.Error != "" || result.Malformed || !result.Installed {
			t.Fatalf("install = %+v", result)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "hooks.json")); !os.IsNotExist(err) {
		t.Fatalf("codex hooks.json should not be created when inline hooks exist: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(data), codexInlineBegin); got != 1 {
		t.Fatalf("managed codex inline hook marker count = %d, want 1\n%s", got, data)
	}
}

func assertManagedClaudeLikeHookCount(t *testing.T, path, event string, want int) {
	t.Helper()
	var root map[string]any
	readJSONFile(t, path, &root)
	hooks := root["hooks"].(map[string]any)
	groups := hooks[event].([]any)
	got := 0
	for _, rawGroup := range groups {
		group := rawGroup.(map[string]any)
		for _, rawHook := range group["hooks"].([]any) {
			if managedHookObjectPresent(rawHook.(map[string]any)) {
				got++
			}
		}
	}
	if got != want {
		t.Fatalf("%s %s managed hook count = %d, want %d", path, event, got, want)
	}
}

func assertManagedFlatHookCount(t *testing.T, path, event string, want int) {
	t.Helper()
	var root map[string]any
	readJSONFile(t, path, &root)
	hooks := root["hooks"].(map[string]any)
	entries := hooks[event].([]any)
	got := 0
	for _, rawHook := range entries {
		if managedHookObjectPresent(rawHook.(map[string]any)) {
			got++
		}
	}
	if got != want {
		t.Fatalf("%s %s managed hook count = %d, want %d", path, event, got, want)
	}
}

func readJSONFile(t *testing.T, path string, out any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("unmarshal %s: %v\n%s", path, err, data)
	}
}

func mustMarshalString(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
