package cli

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gptcmd "github.com/gptscript-ai/cmd"
	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/skillformat"
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

func TestSkillsInstallExactIDInstallsClaudeCode(t *testing.T) {
	home := useSetupTestHome(t)
	server := skillInstallTestServer(t, []skillInstallTestResponse{{
		ID:       "sk1",
		Name:     "github-review",
		Download: skillTestZip(t, "github-review", "Review GitHub pull requests."),
	}})
	defer server.Close()

	stdout, err := executeSkillsTestCommand(skillsTestRoot(server.URL), "install", "sk1", "--agent", "claude-code")
	if err != nil {
		t.Fatal(err)
	}

	assertFileContains(t, filepath.Join(home, ".claude", "skills", "github-review", skillformat.SkillMainFile), "Review GitHub pull requests.")
	if !strings.Contains(stdout, "Installed github-review for Claude Code") {
		t.Fatalf("expected install message, got:\n%s", stdout)
	}
}

func TestSkillsInstallNoMatches(t *testing.T) {
	server := skillInstallTestServer(t, nil)
	defer server.Close()

	_, err := executeSkillsTestCommand(skillsTestRoot(server.URL), "install", "missing", "--agent", "claude-code")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `skill "missing" not found`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSkillsInstallDetectedInstallsClaudeCodeWhenPresent(t *testing.T) {
	home := useSetupTestHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}
	server := skillInstallTestServer(t, []skillInstallTestResponse{{
		ID:       "sk1",
		Name:     "github-review",
		Download: skillTestZip(t, "github-review", "Review GitHub pull requests."),
	}})
	defer server.Close()

	if _, err := executeSkillsTestCommand(skillsTestRoot(server.URL), "install", "sk1"); err != nil {
		t.Fatal(err)
	}

	assertFileContains(t, filepath.Join(home, ".claude", "skills", "github-review", skillformat.SkillMainFile), "Review GitHub pull requests.")
}

func TestSkillsInstallJSONMode(t *testing.T) {
	useSetupTestHome(t)
	server := skillInstallTestServer(t, []skillInstallTestResponse{{
		ID:       "sk1",
		Name:     "github-review",
		Download: skillTestZip(t, "github-review", "Review GitHub pull requests."),
	}})
	defer server.Close()

	stdout, err := executeSkillsTestCommand(skillsTestRoot(server.URL), "install", "sk1", "--agent", "claude-code", "--json")
	if err != nil {
		t.Fatal(err)
	}

	var output skillsInstallOutput
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout)
	}
	if len(output.Results) != 1 {
		t.Fatalf("result count = %d, want 1", len(output.Results))
	}
	if output.Results[0].Agent != "claude-code" || output.Results[0].Mode != "direct" {
		t.Fatalf("unexpected install result: %#v", output.Results[0])
	}
	if len(output.Results[0].Installed) == 0 {
		t.Fatalf("expected installed paths in JSON output")
	}
}

func TestNewIncludesSkillsCommands(t *testing.T) {
	restore := useRootTestEnv(t)
	defer restore()

	root := New()
	if _, _, err := root.Find([]string{"skills", "search"}); err != nil {
		t.Fatalf("skills search command was not registered: %v", err)
	}
	if _, _, err := root.Find([]string{"skills", "install"}); err != nil {
		t.Fatalf("skills install command was not registered: %v", err)
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

type skillInstallTestResponse struct {
	ID          string
	Name        string
	DisplayName string
	Description string
	Download    []byte
}

func skillInstallTestServer(t *testing.T, skills []skillInstallTestResponse) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization = %q, want bearer token", got)
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/skills":
			query := strings.ToLower(r.URL.Query().Get("q"))
			result := types.SkillList{}
			for _, skill := range skills {
				if query == "" ||
					strings.Contains(strings.ToLower(skill.ID), query) ||
					strings.Contains(strings.ToLower(skill.Name), query) ||
					strings.Contains(strings.ToLower(skill.DisplayName), query) {
					result.Items = append(result.Items, skillTestType(skill))
				}
			}
			_ = json.NewEncoder(w).Encode(result)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/download"):
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/skills/"), "/download")
			for _, skill := range skills {
				if skill.ID == id {
					w.Header().Set("Content-Type", "application/zip")
					_, _ = w.Write(skill.Download)
					return
				}
			}
			http.NotFound(w, r)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/skills/"):
			id := strings.TrimPrefix(r.URL.Path, "/skills/")
			for _, skill := range skills {
				if skill.ID == id {
					_ = json.NewEncoder(w).Encode(skillTestType(skill))
					return
				}
			}
			http.NotFound(w, r)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
}

func skillTestType(skill skillInstallTestResponse) types.Skill {
	return types.Skill{
		Metadata: types.Metadata{ID: skill.ID},
		SkillManifest: types.SkillManifest{
			Name:        skill.Name,
			DisplayName: skill.DisplayName,
			Description: skill.Description,
		},
		RepoID: "default/test",
	}
}

func skillTestZip(t *testing.T, name, description string) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	header := &zip.FileHeader{Name: skillformat.SkillMainFile}
	header.SetMode(0644)
	fileWriter, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\nBody\n", name, description)
	if _, err := fileWriter.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func assertFileContains(t *testing.T, path, substr string) {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), substr) {
		t.Fatalf("%s did not contain %q:\n%s", path, substr, content)
	}
}
