package handlers

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/publishedartifact"
	"github.com/obot-platform/obot/pkg/skillformat"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	blobpkg "github.com/obot-platform/obot/pkg/storage/blob"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func createArtifactTestZIP(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatalf("failed to create ZIP entry %s: %v", name, err)
		}
		if _, err := fw.Write(content); err != nil {
			t.Fatalf("failed to write ZIP entry %s: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close ZIP: %v", err)
	}
	return buf.Bytes()
}

func createSkillMDContent(t *testing.T, name, description string, metadata map[string]string) []byte {
	t.Helper()
	fm := skillformat.Frontmatter{
		Name:        name,
		Description: description,
		Metadata:    metadata,
	}
	content, err := skillformat.FormatSkillMD(fm, "")
	if err != nil {
		t.Fatalf("failed to format SKILL.md: %v", err)
	}
	return []byte(content)
}

func TestReadSkillFrontmatterFromZIP(t *testing.T) {
	tests := []struct {
		name        string
		skillName   string
		description string
		metadata    map[string]string
		extraFiles  map[string][]byte
		checkResult func(t *testing.T, fm skillformat.Frontmatter)
	}{
		{
			name:        "valid with all fields",
			skillName:   "test-workflow",
			description: "A test description",
			metadata:    map[string]string{"author-email": "author@test.com"},
			checkResult: func(t *testing.T, fm skillformat.Frontmatter) {
				t.Helper()
				if fm.Name != "test-workflow" {
					t.Errorf("name = %q, want %q", fm.Name, "test-workflow")
				}
				if fm.Description != "A test description" {
					t.Errorf("description = %q, want %q", fm.Description, "A test description")
				}
				if fm.Metadata["author-email"] != "author@test.com" {
					t.Errorf("metadata[author-email] = %q, want %q", fm.Metadata["author-email"], "author@test.com")
				}
			},
		},
		{
			name:        "minimal",
			skillName:   "minimal",
			description: "Minimal skill.",
			checkResult: func(t *testing.T, fm skillformat.Frontmatter) {
				t.Helper()
				if fm.Name != "minimal" {
					t.Errorf("name = %q, want %q", fm.Name, "minimal")
				}
			},
		},
		{
			name:        "with extra files in ZIP",
			skillName:   "with-files",
			description: "Has extra files.",
			extraFiles: map[string][]byte{
				"scripts/analyze.py": []byte("print('hi')"),
			},
			checkResult: func(t *testing.T, fm skillformat.Frontmatter) {
				t.Helper()
				if fm.Name != "with-files" {
					t.Errorf("name = %q, want %q", fm.Name, "with-files")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string][]byte{
				skillformat.SkillMainFile: createSkillMDContent(t, tt.skillName, tt.description, tt.metadata),
			}
			for k, v := range tt.extraFiles {
				files[k] = v
			}

			fm, _, err := readSkillFrontmatterFromZIP(createArtifactTestZIP(t, files))
			if err != nil {
				t.Fatalf("readSkillFrontmatterFromZIP() error: %v", err)
			}
			tt.checkResult(t, fm)
		})
	}
}

func TestReadSkillFrontmatterFromZIP_Errors(t *testing.T) {
	missingSkillZIP := createArtifactTestZIP(t, map[string][]byte{"other.txt": []byte("hello")})
	invalidYAMLZIP := createArtifactTestZIP(t, map[string][]byte{skillformat.SkillMainFile: []byte("---\nbad yaml: [\n---\n")})

	tests := []struct {
		name    string
		data    []byte
		wantErr string
	}{
		{
			name:    "not a zip",
			data:    []byte("not a zip file"),
			wantErr: "invalid ZIP archive",
		},
		{
			name:    "empty bytes",
			data:    []byte{},
			wantErr: "invalid ZIP archive",
		},
		{
			name:    "missing SKILL.md",
			data:    missingSkillZIP,
			wantErr: "SKILL.md not found",
		},
		{
			name:    "invalid yaml in SKILL.md",
			data:    invalidYAMLZIP,
			wantErr: "invalid SKILL.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := readSkillFrontmatterFromZIP(tt.data)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRewriteSkillFrontmatterInZIP(t *testing.T) {
	originalContent := createSkillMDContent(t, "original", "Original desc.", nil)
	otherFileContent := []byte("# Steps\nDo things.")

	zipData := createArtifactTestZIP(t, map[string][]byte{
		skillformat.SkillMainFile: originalContent,
		"scripts/run.sh":          otherFileContent,
	})

	updatedFM := skillformat.Frontmatter{
		Name:        "original",
		Description: "Updated desc.",
		Metadata: map[string]string{
			"id":           "pa1abc123def456",
			"author-email": "injected@example.com",
			"version":      "2",
		},
	}

	result, err := rewriteSkillFrontmatterInZIP(zipData, updatedFM, "")
	if err != nil {
		t.Fatalf("rewriteSkillFrontmatterInZIP() error: %v", err)
	}

	// Verify the frontmatter was updated.
	gotFM, _, err := readSkillFrontmatterFromZIP(result)
	if err != nil {
		t.Fatalf("readSkillFrontmatterFromZIP() on rewritten ZIP error: %v", err)
	}
	if gotFM.Description != "Updated desc." {
		t.Errorf("description = %q, want %q", gotFM.Description, "Updated desc.")
	}
	if gotFM.Metadata["author-email"] != "injected@example.com" {
		t.Errorf("metadata[author-email] = %q, want %q", gotFM.Metadata["author-email"], "injected@example.com")
	}
	if gotFM.Metadata["id"] != "pa1abc123def456" {
		t.Errorf("metadata[id] = %q, want %q", gotFM.Metadata["id"], "pa1abc123def456")
	}
	if gotFM.Metadata["version"] != "2" {
		t.Errorf("metadata[version] = %q, want %q", gotFM.Metadata["version"], "2")
	}

	// Verify other files are preserved.
	r, err := zip.NewReader(bytes.NewReader(result), int64(len(result)))
	if err != nil {
		t.Fatalf("failed to open rewritten ZIP: %v", err)
	}

	fileCount := 0
	for _, f := range r.File {
		fileCount++
		if f.Name == "scripts/run.sh" {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("failed to open scripts/run.sh: %v", err)
			}
			content, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatalf("failed to read scripts/run.sh: %v", err)
			}
			if !bytes.Equal(content, otherFileContent) {
				t.Errorf("scripts/run.sh content = %q, want %q", content, otherFileContent)
			}
		}
	}
	if fileCount != 2 {
		t.Errorf("file count = %d, want 2", fileCount)
	}
}

func TestRewriteSkillFrontmatterInZIP_PreservesMultipleFiles(t *testing.T) {
	files := map[string][]byte{
		skillformat.SkillMainFile: createSkillMDContent(t, "multi", "Multi skill.", nil),
		"scripts/analyze.py":      []byte("print('hello')"),
		"data/config.json":        []byte(`{"key": "value"}`),
	}
	zipData := createArtifactTestZIP(t, files)

	updatedFM := skillformat.Frontmatter{
		Name:        "multi",
		Description: "New description.",
	}

	result, err := rewriteSkillFrontmatterInZIP(zipData, updatedFM, "")
	if err != nil {
		t.Fatalf("rewriteSkillFrontmatterInZIP() error: %v", err)
	}

	r, err := zip.NewReader(bytes.NewReader(result), int64(len(result)))
	if err != nil {
		t.Fatalf("failed to open rewritten ZIP: %v", err)
	}

	found := make(map[string]bool)
	for _, f := range r.File {
		found[f.Name] = true
	}

	for name := range files {
		if !found[name] {
			t.Errorf("file %q missing from rewritten ZIP", name)
		}
	}
	if len(found) != 3 {
		t.Errorf("file count = %d, want 3", len(found))
	}
}

func TestRewriteSkillFrontmatterInZIP_InvalidZIP(t *testing.T) {
	_, err := rewriteSkillFrontmatterInZIP([]byte("not a zip"), skillformat.Frontmatter{}, "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid ZIP archive") {
		t.Errorf("error = %q, want containing %q", err.Error(), "invalid ZIP archive")
	}
}

func TestWithArtifactMetadata(t *testing.T) {
	t.Run("adds publish metadata to empty map", func(t *testing.T) {
		fm := withArtifactMetadata(skillformat.Frontmatter{
			Name:        "test-skill",
			Description: "A test skill.",
		}, "pa1abc123def456", "author@example.com", 3)

		if fm.Metadata["id"] != "pa1abc123def456" {
			t.Errorf("metadata[id] = %q, want %q", fm.Metadata["id"], "pa1abc123def456")
		}
		if fm.Metadata["author-email"] != "author@example.com" {
			t.Errorf("metadata[author-email] = %q, want %q", fm.Metadata["author-email"], "author@example.com")
		}
		if fm.Metadata["version"] != "3" {
			t.Errorf("metadata[version] = %q, want %q", fm.Metadata["version"], "3")
		}
	})

	t.Run("preserves existing metadata", func(t *testing.T) {
		original := skillformat.Frontmatter{
			Name:        "test-skill",
			Description: "A test skill.",
			Metadata: map[string]string{
				"custom": "value",
			},
		}

		fm := withArtifactMetadata(original, "pa1zzz999yyy888", "author@example.com", 1)

		if fm.Metadata["custom"] != "value" {
			t.Errorf("metadata[custom] = %q, want %q", fm.Metadata["custom"], "value")
		}
		if fm.Metadata["id"] != "pa1zzz999yyy888" {
			t.Errorf("metadata[id] = %q, want %q", fm.Metadata["id"], "pa1zzz999yyy888")
		}
		if original.Metadata["id"] != "" {
			t.Errorf("original metadata should not be mutated, got id=%q", original.Metadata["id"])
		}
	})
}

func TestConvertPublishedArtifact(t *testing.T) {
	input := &v1.PublishedArtifact{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pa1abc123",
		},
		Spec: v1.PublishedArtifactSpec{
			PublishedArtifactManifest: types.PublishedArtifactManifest{
				Name:         "my-workflow",
				Description:  "A description",
				ArtifactType: types.PublishedArtifactTypeWorkflow,
				AuthorEmail:  "author@test.com",
			},
			AuthorID:      "user-123",
			LatestVersion: 3,
		},
		Status: v1.PublishedArtifactStatus{
			Versions: []types.PublishedArtifactVersionEntry{
				{
					Version:     1,
					Description: "v1",
					CreatedAt:   *types.NewTime(metav1.Now().Time),
					Subjects: []types.Subject{
						{Type: types.SubjectTypeUser, ID: "user-a"},
					},
				},
				{
					Version:     3,
					Description: "v3",
					CreatedAt:   *types.NewTime(metav1.Now().Time),
					Subjects: []types.Subject{
						{Type: types.SubjectTypeSelector, ID: "*"},
					},
				},
			},
		},
	}

	result := convertPublishedArtifactForRequester(input, &kuser.DefaultInfo{UID: "user-123"}, false)

	if result.ID != "pa1abc123" {
		t.Errorf("ID = %q, want %q", result.ID, "pa1abc123")
	}
	if result.Name != "my-workflow" {
		t.Errorf("Name = %q, want %q", result.Name, "my-workflow")
	}
	if result.Description != "A description" {
		t.Errorf("Description = %q, want %q", result.Description, "A description")
	}
	if result.ArtifactType != types.PublishedArtifactTypeWorkflow {
		t.Errorf("ArtifactType = %q, want %q", result.ArtifactType, types.PublishedArtifactTypeWorkflow)
	}
	if result.AuthorEmail != "author@test.com" {
		t.Errorf("AuthorEmail = %q, want %q", result.AuthorEmail, "author@test.com")
	}
	if result.AuthorID != "user-123" {
		t.Errorf("AuthorID = %q, want %q", result.AuthorID, "user-123")
	}
	if result.LatestVersion != 3 {
		t.Errorf("LatestVersion = %d, want 3", result.LatestVersion)
	}
	if len(result.Versions) != 2 {
		t.Fatalf("Versions = %+v, want 2 versions", result.Versions)
	}
	var version3 *types.PublishedArtifactVersionSummary
	for i := range result.Versions {
		if result.Versions[i].Version == 3 {
			version3 = &result.Versions[i]
			break
		}
	}
	if version3 == nil {
		t.Fatalf("Versions = %+v, want version 3 present", result.Versions)
	}
	if len(version3.Subjects) != 1 || version3.Subjects[0] != (types.Subject{Type: types.SubjectTypeSelector, ID: "*"}) {
		t.Errorf("Version 3 Subjects = %+v, want wildcard selector", version3.Subjects)
	}
	if result.Type != "publishedartifact" {
		t.Errorf("Type = %q, want %q", result.Type, "publishedartifact")
	}
}

func TestConvertPublishedArtifact_ZeroValue(t *testing.T) {
	input := &v1.PublishedArtifact{}
	result := convertPublishedArtifactForRequester(input, &kuser.DefaultInfo{UID: "user-123"}, false)

	if result.Name != "" {
		t.Errorf("Name = %q, want empty", result.Name)
	}
	if result.LatestVersion != 0 {
		t.Errorf("LatestVersion = %d, want 0", result.LatestVersion)
	}
}

func TestValidatePublishedArtifactSubjects(t *testing.T) {
	tests := []struct {
		name     string
		subjects []types.Subject
		wantErr  string
	}{
		{
			name: "valid user and group",
			subjects: []types.Subject{
				{Type: types.SubjectTypeUser, ID: "user1"},
				{Type: types.SubjectTypeGroup, ID: "group1"},
			},
		},
		{
			name: "valid wildcard",
			subjects: []types.Subject{
				{Type: types.SubjectTypeSelector, ID: "*"},
			},
		},
		{
			name: "wildcard mixed",
			subjects: []types.Subject{
				{Type: types.SubjectTypeSelector, ID: "*"},
				{Type: types.SubjectTypeUser, ID: "user1"},
			},
			wantErr: "wildcard subject",
		},
		{
			name: "wildcard non selector",
			subjects: []types.Subject{
				{Type: types.SubjectTypeUser, ID: "*"},
			},
			wantErr: "wildcard subject (*) must use selector type",
		},
		{
			name: "duplicate subject",
			subjects: []types.Subject{
				{Type: types.SubjectTypeUser, ID: "user1"},
				{Type: types.SubjectTypeUser, ID: "user1"},
			},
			wantErr: "duplicate subject",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePublishedArtifactSubjects(tt.subjects)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want containing %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestCanAccessArtifact(t *testing.T) {
	artifact := &v1.PublishedArtifact{
		Spec: v1.PublishedArtifactSpec{
			AuthorID: "owner",
		},
		Status: v1.PublishedArtifactStatus{
			Versions: []types.PublishedArtifactVersionEntry{
				{
					Version: 1,
					Subjects: []types.Subject{
						{Type: types.SubjectTypeGroup, ID: "group1"},
					},
				},
			},
		},
	}

	if !publishedartifact.CanAccess(artifact, &kuser.DefaultInfo{UID: "owner"}, false) {
		t.Fatal("owner should have access")
	}
	if !publishedartifact.CanAccess(artifact, &kuser.DefaultInfo{UID: "other"}, true) {
		t.Fatal("admin should have access")
	}
	if !publishedartifact.CanAccess(artifact, &kuser.DefaultInfo{
		UID: "other",
		Extra: map[string][]string{
			"auth_provider_groups": {"group1"},
		},
	}, false) {
		t.Fatal("group member should have access")
	}
	if publishedartifact.CanAccess(artifact, &kuser.DefaultInfo{UID: "other"}, false) {
		t.Fatal("unmatched user should not have access")
	}
}

func TestConvertPublishedArtifactForRequester_FiltersVersions(t *testing.T) {
	input := &v1.PublishedArtifact{
		Spec: v1.PublishedArtifactSpec{
			PublishedArtifactManifest: types.PublishedArtifactManifest{
				Name: "my-workflow",
			},
			AuthorID:      "owner",
			LatestVersion: 2,
		},
		Status: v1.PublishedArtifactStatus{
			Versions: []types.PublishedArtifactVersionEntry{
				{
					Version:  1,
					Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
				},
				{
					Version:  2,
					Subjects: nil,
				},
			},
		},
	}

	result := convertPublishedArtifactForRequester(input, &kuser.DefaultInfo{UID: "other"}, false)

	if result.LatestVersion != 1 {
		t.Fatalf("LatestVersion = %d, want 1", result.LatestVersion)
	}
	if len(result.Versions) != 1 || result.Versions[0].Version != 1 {
		t.Fatalf("Versions = %+v, want only v1", result.Versions)
	}
}

func newPublishedArtifactTestStorage(t *testing.T, objects ...kclient.Object) kclient.WithWatch {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(objects...).
		Build()
}

func TestPublishedArtifactCreate_InheritsSubjectsFromPreviousVersion(t *testing.T) {
	artifactName := system.PublishedArtifactPrefix + hash.String("owner" + string(types.PublishedArtifactTypeWorkflow) + "workflow-a")[:12]

	storage := newPublishedArtifactTestStorage(t, &v1.PublishedArtifact{
		ObjectMeta: metav1.ObjectMeta{
			Name:      artifactName,
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.PublishedArtifactSpec{
			PublishedArtifactManifest: types.PublishedArtifactManifest{
				Name:         "workflow-a",
				Description:  "v1",
				ArtifactType: types.PublishedArtifactTypeWorkflow,
				AuthorEmail:  "owner@example.com",
			},
			AuthorID:      "owner",
			LatestVersion: 1,
			BlobKey:       "published-artifacts/" + artifactName + "/v1.zip",
		},
		Status: v1.PublishedArtifactStatus{
			Versions: []types.PublishedArtifactVersionEntry{
				{
					Version:     1,
					BlobKey:     "published-artifacts/" + artifactName + "/v1.zip",
					Description: "v1",
					CreatedAt:   *types.NewTime(metav1.Now().Time),
					Subjects: []types.Subject{
						{Type: types.SubjectTypeUser, ID: "user-a"},
						{Type: types.SubjectTypeGroup, ID: "group-b"},
					},
				},
			},
		},
	})

	blobStore, err := blobpkg.NewDirectoryStore(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create directory blob store: %v", err)
	}
	handler := NewPublishedArtifactHandler(blobStore, "test-bucket")
	reqBody := createArtifactTestZIP(t, map[string][]byte{
		skillformat.SkillMainFile: createSkillMDContent(t, "workflow-a", "v2", nil),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/published-artifacts", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()

	err = handler.Create(api.Context{
		ResponseWriter: rec,
		Request:        req,
		Storage:        storage,
		User: &kuser.DefaultInfo{
			Name:   "owner",
			UID:    "owner",
			Groups: []string{types.GroupAuthenticated},
			Extra: map[string][]string{
				"email": {"owner@example.com"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}

	var artifact v1.PublishedArtifact
	if err := storage.Get(context.Background(), kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      artifactName,
	}, &artifact); err != nil {
		t.Fatalf("failed to fetch updated artifact: %v", err)
	}

	if artifact.Spec.LatestVersion != 2 {
		t.Fatalf("LatestVersion = %d, want 2", artifact.Spec.LatestVersion)
	}
	if len(artifact.Status.Versions) != 2 {
		t.Fatalf("Versions len = %d, want 2", len(artifact.Status.Versions))
	}

	gotSubjects := artifact.Status.Versions[1].Subjects
	wantSubjects := []types.Subject{
		{Type: types.SubjectTypeUser, ID: "user-a"},
		{Type: types.SubjectTypeGroup, ID: "group-b"},
	}
	if len(gotSubjects) != len(wantSubjects) {
		t.Fatalf("v2 subjects len = %d, want %d", len(gotSubjects), len(wantSubjects))
	}
	for i := range wantSubjects {
		if gotSubjects[i] != wantSubjects[i] {
			t.Fatalf("v2 subjects[%d] = %+v, want %+v", i, gotSubjects[i], wantSubjects[i])
		}
	}

	var response types.PublishedArtifact
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(response.Versions) != 2 {
		t.Fatalf("response versions len = %d, want 2", len(response.Versions))
	}
	if len(response.Versions[1].Subjects) != len(wantSubjects) {
		t.Fatalf("response v2 subjects len = %d, want %d", len(response.Versions[1].Subjects), len(wantSubjects))
	}
}

func TestValidateZIP_PathTraversal(t *testing.T) {
	zipData := createArtifactTestZIP(t, map[string][]byte{
		"../etc/passwd": []byte("bad"),
	})
	r, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("failed to create zip reader: %v", err)
	}
	if err := validateZIP(r); err == nil {
		t.Fatal("expected error for path traversal entry")
	} else if !strings.Contains(err.Error(), "path traversal") {
		t.Errorf("error = %q, want containing 'path traversal'", err.Error())
	}
}

func TestValidateZIP_AbsolutePath(t *testing.T) {
	zipData := createArtifactTestZIP(t, map[string][]byte{
		"/etc/passwd": []byte("bad"),
	})
	r, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("failed to create zip reader: %v", err)
	}
	if err := validateZIP(r); err == nil {
		t.Fatal("expected error for absolute path entry")
	} else if !strings.Contains(err.Error(), "absolute path") {
		t.Errorf("error = %q, want containing 'absolute path'", err.Error())
	}
}

func TestValidateZIP_TooManyFiles(t *testing.T) {
	files := make(map[string][]byte)
	for i := range maxZIPFiles + 1 {
		files[fmt.Sprintf("file%d.txt", i)] = []byte("x")
	}
	zipData := createArtifactTestZIP(t, files)
	r, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("failed to create zip reader: %v", err)
	}
	if err := validateZIP(r); err == nil {
		t.Fatal("expected error for too many files")
	} else if !strings.Contains(err.Error(), "too many files") {
		t.Errorf("error = %q, want containing 'too many files'", err.Error())
	}
}

func TestValidateZIP_Valid(t *testing.T) {
	zipData := createArtifactTestZIP(t, map[string][]byte{
		skillformat.SkillMainFile: createSkillMDContent(t, "test", "A test.", nil),
		"scripts/run.sh":          []byte("#!/bin/bash"),
	})
	r, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("failed to create zip reader: %v", err)
	}
	if err := validateZIP(r); err != nil {
		t.Fatalf("validateZIP() unexpected error: %v", err)
	}
}

func TestValidateZIPEntryName(t *testing.T) {
	tests := []struct {
		name    string
		entry   string
		wantErr string
	}{
		{name: "valid simple", entry: "SKILL.md"},
		{name: "valid nested", entry: "scripts/run.sh"},
		{name: "absolute unix", entry: "/etc/passwd", wantErr: "absolute path"},
		{name: "traversal unix", entry: "../etc/passwd", wantErr: "path traversal"},
		{name: "traversal nested", entry: "foo/../../etc/passwd", wantErr: "path traversal"},
		{name: "windows backslash traversal", entry: `..\..\evil.sh`, wantErr: "path traversal"},
		{name: "windows drive letter", entry: `C:\tmp\evil.sh`, wantErr: "absolute path"},
		{name: "windows drive slash", entry: "C:/tmp/evil.sh", wantErr: "absolute path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateZIPEntryName(tt.entry)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRewriteSkillFrontmatterInZIP_EnforcesActualSize(t *testing.T) {
	// Build a ZIP where we stuff content close to the limit to test that actual
	// bytes are tracked. We use the Store method (no compression) so what we
	// write is what gets decompressed.
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	// SKILL.md first
	skillContent := createSkillMDContent(t, "big", "Big skill.", nil)
	fw, err := w.Create(skillformat.SkillMainFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(skillContent); err != nil {
		t.Fatal(err)
	}

	// A big file that pushes total past the limit.
	header := &zip.FileHeader{Name: "big.bin", Method: zip.Store}
	bw, err := w.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	chunk := make([]byte, 1024*1024) // 1 MB
	for written := 0; written <= maxZIPUncompressedBytes; written += len(chunk) {
		if _, err := bw.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	fm := skillformat.Frontmatter{Name: "big", Description: "Big skill."}
	_, err = rewriteSkillFrontmatterInZIP(buf.Bytes(), fm, "")
	if err == nil {
		t.Fatal("expected error for oversized content during rewrite")
	}
	if !strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("unexpected error: %v", err)
	}
}
