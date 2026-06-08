package localagents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditHookFixturesAreValidJSON(t *testing.T) {
	fixturesRoot := filepath.Join("testdata", "audit-hooks")
	wantClients := map[string]int{
		"claude-code": 4,
		"codex-cli":   4,
		"cursor":      4,
	}
	gotClients := map[string]int{}

	err := filepath.WalkDir(fixturesRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}

		rel, err := filepath.Rel(fixturesRoot, path)
		if err != nil {
			return err
		}
		client, _, _ := strings.Cut(rel, string(filepath.Separator))
		gotClients[client]++

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var payload map[string]any
		if err := json.Unmarshal(content, &payload); err != nil {
			t.Fatalf("%s is not valid JSON: %v", path, err)
		}

		for _, field := range []string{"session_id", "cwd"} {
			if _, ok := payload[field]; !ok {
				t.Fatalf("%s missing common field %q", path, field)
			}
		}
		if _, ok := payload["hook_event_name"]; !ok {
			if _, ok := payload["hookEventName"]; !ok {
				t.Fatalf("%s missing hook event name field", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for client, want := range wantClients {
		if got := gotClients[client]; got != want {
			t.Fatalf("fixture count for %s = %d, want %d", client, got, want)
		}
	}
}
