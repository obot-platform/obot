package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gptcmd "github.com/gptscript-ai/cmd"
	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
)

func TestSkillsSearchCallsAPIWithQueryAndLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/skills" {
			t.Fatalf("path = %s, want /skills", r.URL.Path)
		}
		if got := r.URL.Query().Get("q"); got != "github" {
			t.Fatalf("q = %q, want github", got)
		}
		if got := r.URL.Query().Get("limit"); got != "7" {
			t.Fatalf("limit = %q, want 7", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization = %q, want bearer token", got)
		}

		_ = json.NewEncoder(w).Encode(types.SkillList{Items: []types.Skill{{
			Metadata: types.Metadata{ID: "sk1"},
			SkillManifest: types.SkillManifest{
				Name:          "github-review",
				DisplayName:   "GitHub Review",
				Description:   "Review GitHub pull requests",
				Compatibility: "claude-code",
			},
			RepoID: "default/github",
		}}})
	}))
	defer server.Close()

	stdout, err := executeSkillsTestCommand(skillsTestRoot(server.URL), "search", "github", "--limit", "7")
	if err != nil {
		t.Fatal(err)
	}

	output := stdout
	for _, want := range []string{"ID", "NAME", "DESCRIPTION", "REPOSITORY", "COMPATIBILITY", "sk1", "GitHub Review", "default/github", "claude-code"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, output)
		}
	}
}

func TestSkillsSearchEmptyResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(types.SkillList{})
	}))
	defer server.Close()

	stdout, err := executeSkillsTestCommand(skillsTestRoot(server.URL), "search")
	if err != nil {
		t.Fatal(err)
	}

	if got := stdout; !strings.Contains(got, "No skills found") {
		t.Fatalf("expected empty message, got:\n%s", got)
	}
}

func TestSkillsSearchJSONMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(types.SkillList{Items: []types.Skill{{
			Metadata:      types.Metadata{ID: "sk1"},
			SkillManifest: types.SkillManifest{Name: "github-review"},
		}}})
	}))
	defer server.Close()

	stdout, err := executeSkillsTestCommand(skillsTestRoot(server.URL), "search", "--json")
	if err != nil {
		t.Fatal(err)
	}

	var result types.SkillList
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout)
	}
	if len(result.Items) != 1 || result.Items[0].ID != "sk1" || result.Items[0].Name != "github-review" {
		t.Fatalf("unexpected JSON result: %#v", result)
	}
}

func TestNewIncludesSkillsSearchCommand(t *testing.T) {
	restore := useRootTestEnv(t)
	defer restore()

	root := New()
	if _, _, err := root.Find([]string{"skills", "search"}); err != nil {
		t.Fatalf("skills search command was not registered: %v", err)
	}
}

func skillsTestRoot(baseURL string) *Obot {
	return &Obot{Client: &apiclient.Client{
		BaseURL: baseURL,
		Token:   "test-token",
	}}
}

func executeSkillsTestCommand(root *Obot, args ...string) (string, error) {
	var stdout bytes.Buffer
	cmd := gptcmd.Command(&Skills{root: root})
	cmd.SetContext(context.Background())
	cmd.SetOut(&stdout)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), err
}
