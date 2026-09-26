package skillformat

import (
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expectErr bool
	}{
		{
			name:      "valid simple",
			input:     "my-skill",
			expectErr: false,
		},
		{
			name:      "valid single char",
			input:     "a",
			expectErr: false,
		},
		{
			name:      "valid with numbers",
			input:     "skill-123",
			expectErr: false,
		},
		{
			name:      "valid all numbers",
			input:     "123",
			expectErr: false,
		},
		{
			name:      "valid max length",
			input:     strings.Repeat("a", 64),
			expectErr: false,
		},
		{
			name:      "empty",
			input:     "",
			expectErr: true,
		},
		{
			name:      "too long",
			input:     strings.Repeat("a", 65),
			expectErr: true,
		},
		{
			name:      "uppercase",
			input:     "My-Skill",
			expectErr: true,
		},
		{
			name:      "leading hyphen",
			input:     "-leading",
			expectErr: true,
		},
		{
			name:      "trailing hyphen",
			input:     "trailing-",
			expectErr: true,
		},
		{
			name:      "consecutive hyphens",
			input:     "double--hyphen",
			expectErr: true,
		},
		{
			name:      "space",
			input:     "has space",
			expectErr: true,
		},
		{
			name:      "underscore",
			input:     "has_underscore",
			expectErr: true,
		},
		{
			name:      "dot",
			input:     "has.dot",
			expectErr: true,
		},
		{
			name:      "only hyphen",
			input:     "-",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.input)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateName(%q) error = %v, expectErr = %v", tt.input, err, tt.expectErr)
			}
		})
	}
}

func TestValidateDescription(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expectErr bool
	}{
		{
			name:      "valid",
			input:     "A valid description",
			expectErr: false,
		},
		{
			name:      "valid max length",
			input:     strings.Repeat("x", 1024),
			expectErr: false,
		},
		{
			name:      "empty",
			input:     "",
			expectErr: true,
		},
		{
			name:      "too long",
			input:     strings.Repeat("x", 1025),
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDescription(tt.input)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateDescription(%q...) error = %v, expectErr = %v", tt.input[:min(len(tt.input), 20)], err, tt.expectErr)
			}
		})
	}
}

func TestValidateFrontmatter(t *testing.T) {
	tests := []struct {
		name      string
		fm        Frontmatter
		expectErr bool
	}{
		{
			name: "valid required only",
			fm: Frontmatter{
				Name:        "my-skill",
				Description: "A test skill.",
			},
			expectErr: false,
		},
		{
			name: "valid with optional fields",
			fm: Frontmatter{
				Name:          "my-skill",
				Description:   "A test skill.",
				License:       "MIT",
				Compatibility: "Requires Python 3.10+",
				Metadata:      map[string]string{"author": "test"},
				AllowedTools:  "Bash(git:*) Read",
			},
			expectErr: false,
		},
		{
			name: "missing name",
			fm: Frontmatter{
				Description: "A test skill.",
			},
			expectErr: true,
		},
		{
			name: "missing description",
			fm: Frontmatter{
				Name: "my-skill",
			},
			expectErr: true,
		},
		{
			name:      "missing both",
			fm:        Frontmatter{},
			expectErr: true,
		},
		{
			name: "invalid name format",
			fm: Frontmatter{
				Name:        "My Skill",
				Description: "A test skill.",
			},
			expectErr: true,
		},
		{
			name: "compatibility too long",
			fm: Frontmatter{
				Name:          "my-skill",
				Description:   "A test skill.",
				Compatibility: strings.Repeat("x", 501),
			},
			expectErr: true,
		},
		{
			name: "compatibility at max",
			fm: Frontmatter{
				Name:          "my-skill",
				Description:   "A test skill.",
				Compatibility: strings.Repeat("x", 500),
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFrontmatter(tt.fm)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateFrontmatter() error = %v, expectErr = %v", err, tt.expectErr)
			}
		})
	}
}

func TestValidateFrontmatter_MultipleErrors(t *testing.T) {
	err := ValidateFrontmatter(Frontmatter{})
	if err == nil {
		t.Fatal("expected error for empty frontmatter")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "name") {
		t.Errorf("expected error to mention name, got: %s", errStr)
	}
	if !strings.Contains(errStr, "description") {
		t.Errorf("expected error to mention description, got: %s", errStr)
	}
}

func TestValidateNameMatchesDir(t *testing.T) {
	tests := []struct {
		name      string
		fmName    string
		dirName   string
		expectErr bool
	}{
		{
			name:      "match",
			fmName:    "my-skill",
			dirName:   "my-skill",
			expectErr: false,
		},
		{
			name:      "mismatch",
			fmName:    "my-skill",
			dirName:   "other-skill",
			expectErr: true,
		},
		{
			name:      "case mismatch",
			fmName:    "my-skill",
			dirName:   "My-Skill",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNameMatchesDir(tt.fmName, tt.dirName)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateNameMatchesDir(%q, %q) error = %v, expectErr = %v", tt.fmName, tt.dirName, err, tt.expectErr)
			}
		})
	}
}

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantFM    Frontmatter
		wantBody  string
		expectErr bool
	}{
		{
			name:    "valid with body",
			content: "---\nname: my-skill\ndescription: A test skill.\n---\n# Hello\nBody content.",
			wantFM: Frontmatter{
				Name:        "my-skill",
				Description: "A test skill.",
			},
			wantBody: "# Hello\nBody content.",
		},
		{
			name:     "no frontmatter",
			content:  "# Just a markdown file\nNo frontmatter here.",
			wantFM:   Frontmatter{},
			wantBody: "# Just a markdown file\nNo frontmatter here.",
		},
		{
			name:      "missing closing delimiter",
			content:   "---\nname: my-skill\ndescription: test\n# No closing",
			expectErr: true,
		},
		{
			name:      "invalid yaml",
			content:   "---\n: :\n  bad:\n    - [\n---\nBody",
			expectErr: true,
		},
		{
			name:    "with metadata",
			content: "---\nname: my-skill\ndescription: A skill.\nmetadata:\n  createdAt: \"2026-01-15T09:00:00Z\"\n  author: test\n---\nBody.",
			wantFM: Frontmatter{
				Name:        "my-skill",
				Description: "A skill.",
				Metadata: map[string]string{
					"createdAt": "2026-01-15T09:00:00Z",
					"author":    "test",
				},
			},
			wantBody: "Body.",
		},
		{
			name:    "empty body after frontmatter",
			content: "---\nname: my-skill\ndescription: test.\n---",
			wantFM: Frontmatter{
				Name:        "my-skill",
				Description: "test.",
			},
			wantBody: "",
		},
		{
			name:     "empty content",
			content:  "",
			wantFM:   Frontmatter{},
			wantBody: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, body, err := ParseFrontmatter(tt.content)
			if (err != nil) != tt.expectErr {
				t.Fatalf("ParseFrontmatter() error = %v, expectErr = %v", err, tt.expectErr)
			}
			if tt.expectErr {
				return
			}
			if fm.Name != tt.wantFM.Name {
				t.Errorf("Name = %q, want %q", fm.Name, tt.wantFM.Name)
			}
			if fm.Description != tt.wantFM.Description {
				t.Errorf("Description = %q, want %q", fm.Description, tt.wantFM.Description)
			}
			if body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
			if tt.wantFM.Metadata != nil {
				for k, v := range tt.wantFM.Metadata {
					if fm.Metadata[k] != v {
						t.Errorf("Metadata[%q] = %q, want %q", k, fm.Metadata[k], v)
					}
				}
			}
		})
	}
}

func TestParseAndValidateFrontmatter(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		expectErr bool
	}{
		{
			name:    "valid",
			content: "---\nname: my-skill\ndescription: A test skill.\n---\nBody.",
		},
		{
			name:      "valid parse but invalid name",
			content:   "---\nname: My Skill\ndescription: A test skill.\n---\nBody.",
			expectErr: true,
		},
		{
			name:      "parse error",
			content:   "---\nbad yaml: [\n---\n",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ParseAndValidateFrontmatter(tt.content)
			if (err != nil) != tt.expectErr {
				t.Errorf("ParseAndValidateFrontmatter() error = %v, expectErr = %v", err, tt.expectErr)
			}
		})
	}
}

func TestFormatSkillMD(t *testing.T) {
	tests := []struct {
		name string
		fm   Frontmatter
		body string
	}{
		{
			name: "basic",
			fm: Frontmatter{
				Name:        "my-skill",
				Description: "A test skill.",
			},
			body: "# Hello\nBody content.",
		},
		{
			name: "with metadata",
			fm: Frontmatter{
				Name:        "my-skill",
				Description: "A test skill.",
				Metadata:    map[string]string{"author-email": "test@example.com"},
			},
			body: "# Hello",
		},
		{
			name: "empty body",
			fm: Frontmatter{
				Name:        "my-skill",
				Description: "A test skill.",
			},
			body: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FormatSkillMD(tt.fm, tt.body)
			if err != nil {
				t.Fatalf("FormatSkillMD() error: %v", err)
			}
			if !strings.HasPrefix(result, "---\n") {
				t.Error("result should start with ---")
			}
			// Round-trip: parse the result and verify it matches
			gotFM, gotBody, err := ParseFrontmatter(result)
			if err != nil {
				t.Fatalf("round-trip ParseFrontmatter() error: %v", err)
			}
			if gotFM.Name != tt.fm.Name {
				t.Errorf("round-trip Name = %q, want %q", gotFM.Name, tt.fm.Name)
			}
			if gotFM.Description != tt.fm.Description {
				t.Errorf("round-trip Description = %q, want %q", gotFM.Description, tt.fm.Description)
			}
			if gotBody != tt.body {
				t.Errorf("round-trip body = %q, want %q", gotBody, tt.body)
			}
			for k, v := range tt.fm.Metadata {
				if gotFM.Metadata[k] != v {
					t.Errorf("round-trip Metadata[%q] = %q, want %q", k, gotFM.Metadata[k], v)
				}
			}
		})
	}
}
