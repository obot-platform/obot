package handlers

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/skillformat"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kuser "k8s.io/apiserver/pkg/authentication/user"
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
			Visibility:    types.PublishedArtifactVisibilityPublic,
		},
	}

	result := convertPublishedArtifact(input)

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
	if result.Visibility != types.PublishedArtifactVisibilityPublic {
		t.Errorf("Visibility = %q, want %q", result.Visibility, types.PublishedArtifactVisibilityPublic)
	}
	if result.Type != "publishedartifact" {
		t.Errorf("Type = %q, want %q", result.Type, "publishedartifact")
	}
}

func TestConvertPublishedArtifact_ZeroValue(t *testing.T) {
	input := &v1.PublishedArtifact{}
	result := convertPublishedArtifact(input)

	if result.Name != "" {
		t.Errorf("Name = %q, want empty", result.Name)
	}
	if result.LatestVersion != 0 {
		t.Errorf("LatestVersion = %d, want 0", result.LatestVersion)
	}
	if result.Visibility != "" {
		t.Errorf("Visibility = %q, want empty", result.Visibility)
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

func TestValidateZIP_Symlink(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	header := &zip.FileHeader{Name: "scripts/link", Method: zip.Store}
	header.SetMode(os.ModeSymlink | 0o777)

	fw, err := w.CreateHeader(header)
	if err != nil {
		t.Fatalf("failed to create symlink entry: %v", err)
	}
	if _, err := fw.Write([]byte("../outside")); err != nil {
		t.Fatalf("failed to write symlink entry: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close zip: %v", err)
	}

	r, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("failed to create zip reader: %v", err)
	}
	if err := validateZIP(r); err == nil {
		t.Fatal("expected error for symlink entry")
	} else if !strings.Contains(err.Error(), "unsupported file type") {
		t.Errorf("error = %q, want containing 'unsupported file type'", err.Error())
	}
}

func TestCheckOwnership_ReturnsNotFoundForNonOwner(t *testing.T) {
	req := httptest.NewRequest(http.MethodPatch, "/api/published-artifacts/pa123", nil)
	req.SetPathValue("id", "pa123")

	err := (&PublishedArtifactHandler{}).checkOwnership(&v1.PublishedArtifact{
		Spec: v1.PublishedArtifactSpec{
			AuthorID: "owner",
		},
	}, api.Context{
		Request:        req,
		ResponseWriter: httptest.NewRecorder(),
		User: &kuser.DefaultInfo{
			UID:    "other-user",
			Groups: []string{types.GroupAuthenticated},
		},
	})
	if err == nil {
		t.Fatal("expected error for non-owner")
	}

	errHTTP, ok := err.(*types.ErrHTTP)
	if !ok {
		t.Fatalf("expected *types.ErrHTTP, got %T", err)
	}
	if errHTTP.Code != http.StatusNotFound {
		t.Fatalf("status code = %d, want %d", errHTTP.Code, http.StatusNotFound)
	}
	if !strings.Contains(errHTTP.Message, "pa123") {
		t.Fatalf("message = %q, want artifact id", errHTTP.Message)
	}
}
