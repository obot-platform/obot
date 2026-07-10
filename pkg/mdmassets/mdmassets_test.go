package mdmassets

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fieldsSchema mirrors the fields object obot-sentry's build/manifest.json
// ships: serverURL server-supplied (readOnly + the manifest's `hidden`
// annotation), scanIntervalMinutes bounded with a default. The
// obotSentryVersion render field comes from the manifest top level, not
// from fields.
const fieldsSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["serverURL"],
  "properties": {
    "serverURL": {"type": "string", "format": "uri", "readOnly": true, "hidden": true},
    "scanIntervalMinutes": {"type": "integer", "minimum": 15, "maximum": 1440, "default": 60}
  }
}`

// writeAssets stages a minimal valid assets tree and returns it. The
// "multi" platform targets two OSes to exercise OS disambiguation.
func writeAssets(t *testing.T, schema string) string {
	t.Helper()
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "windows", "intune", "obot-sentry.intunewin"), "fake-intunewin")
	mustWrite(t, filepath.Join(dir, "windows", "intune", "INSTRUCTIONS.md.tmpl"),
		"# Setup\nserver={{.serverURL}} interval={{.scanIntervalMinutes}} version={{.obotSentryVersion}}\n")
	mustWrite(t, filepath.Join(dir, "macos", "obot-sentry.pkg"), "fake-pkg")
	mustWrite(t, filepath.Join(dir, "macos", "INSTRUCTIONS.md.tmpl"), "# macOS setup\n")
	mustWrite(t, filepath.Join(dir, "icons", "intune.svg"), "<svg/>")

	manifest := `{
  "schemaVersion": "` + schema + `",
  "obotSentryVersion": "1.2.3",
  "fields": ` + fieldsSchema + `,
  "platforms": [
    {"id": "intune", "label": "Microsoft Intune", "icon": "icons/intune.svg"},
    {"id": "multi", "label": "Multi"}
  ],
  "configurations": [
    {
      "platform": "intune",
      "os": "windows",
      "osLabel": "Windows",
      "description": "Enroll your Windows fleet.",
      "suggestedName": "Windows Fleet",
      "instructions": "windows/intune/INSTRUCTIONS.md.tmpl",
      "assets": [
        "windows/intune/obot-sentry.intunewin",
        "windows/intune/INSTRUCTIONS.md.tmpl"
      ]
    },
    {
      "platform": "multi",
      "os": "windows",
      "osLabel": "Windows",
      "description": "d",
      "suggestedName": "n",
      "instructions": "windows/intune/INSTRUCTIONS.md.tmpl",
      "assets": ["windows/intune/INSTRUCTIONS.md.tmpl"]
    },
    {
      "platform": "multi",
      "os": "macos",
      "osLabel": "macOS",
      "description": "d",
      "suggestedName": "n",
      "instructions": "macos/INSTRUCTIONS.md.tmpl",
      "assets": ["macos/obot-sentry.pkg", "macos/INSTRUCTIONS.md.tmpl"]
    }
  ]
}`
	mustWrite(t, filepath.Join(dir, "manifest.json"), manifest)
	return dir
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNewFSRejectsUnknownSchema(t *testing.T) {
	if _, err := NewFS(os.DirFS(writeAssets(t, "v999"))); err == nil {
		t.Error("unknown schemaVersion should be rejected")
	}
}

// Structural problems must fail at startup, not at download time.
func TestNewRejectsBrokenTrees(t *testing.T) {
	breakages := map[string]func(dir, manifest string) string{
		"missing file": func(dir, manifest string) string {
			if err := os.Remove(filepath.Join(dir, "windows", "intune", "obot-sentry.intunewin")); err != nil {
				t.Fatal(err)
			}
			return manifest
		},
		"missing icon": func(dir, manifest string) string {
			if err := os.Remove(filepath.Join(dir, "icons", "intune.svg")); err != nil {
				t.Fatal(err)
			}
			return manifest
		},
		"dangling platform reference": func(_, manifest string) string {
			return strings.Replace(manifest, `"platform": "intune",`, `"platform": "unknown",`, 1)
		},
		"duplicate platform": func(_, manifest string) string {
			return strings.Replace(manifest, `{"id": "multi", "label": "Multi"}`, `{"id": "intune", "label": "Again"}`, 1)
		},
		"duplicate unit": func(_, manifest string) string {
			return strings.Replace(manifest, `"platform": "multi",
      "os": "macos",`, `"platform": "multi",
      "os": "windows",`, 1)
		},
		"instructions not in assets": func(_, manifest string) string {
			// Points at a file that exists on disk but is not part of this
			// configuration's assets, so it wouldn't ship in the download.
			return strings.Replace(manifest, `"instructions": "macos/INSTRUCTIONS.md.tmpl",`, `"instructions": "windows/intune/INSTRUCTIONS.md.tmpl",`, 1)
		},
		"invalid template": func(dir, manifest string) string {
			mustWrite(t, filepath.Join(dir, "windows", "intune", "INSTRUCTIONS.md.tmpl"), "{{")
			return manifest
		},
		"invalid schema default": func(_, manifest string) string {
			return strings.Replace(manifest, `"default": 60`, `"default": "invalid"`, 1)
		},
		"duplicate asset": func(_, manifest string) string {
			return strings.Replace(manifest, `"windows/intune/obot-sentry.intunewin",
        "windows/intune/INSTRUCTIONS.md.tmpl"`, `"windows/intune/INSTRUCTIONS.md.tmpl",
        "windows/intune/INSTRUCTIONS.md.tmpl"`, 1)
		},
		"duplicate flattened output": func(dir, manifest string) string {
			mustWrite(t, filepath.Join(dir, "other", "INSTRUCTIONS.md.tmpl"), "valid")
			return strings.Replace(manifest, `"windows/intune/obot-sentry.intunewin",`, `"other/INSTRUCTIONS.md.tmpl",`, 1)
		},
		"empty platform id": func(_, manifest string) string {
			return strings.Replace(manifest, `{"id": "intune"`, `{"id": ""`, 1)
		},
		"empty obot-sentry version": func(_, manifest string) string {
			return strings.Replace(manifest, `"obotSentryVersion": "1.2.3"`, `"obotSentryVersion": ""`, 1)
		},
		"empty os": func(_, manifest string) string {
			return strings.Replace(manifest, `"os": "windows",`, `"os": "",`, 1)
		},
	}
	for name, breakage := range breakages {
		t.Run(name, func(t *testing.T) {
			dir := writeAssets(t, SchemaVersion)
			raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			mustWrite(t, filepath.Join(dir, "manifest.json"), breakage(dir, string(raw)))
			if _, err := NewFS(os.DirFS(dir)); err == nil {
				t.Errorf("%s should be rejected", name)
			}
		})
	}
}

func TestManifestCarriedThrough(t *testing.T) {
	l, err := NewFS(os.DirFS(writeAssets(t, SchemaVersion)))
	if err != nil {
		t.Fatal(err)
	}
	manifest := l.Manifest()
	if manifest.ObotSentryVersion != "1.2.3" {
		t.Fatalf("loader not usable: version=%q", manifest.ObotSentryVersion)
	}
	platforms := manifest.Platforms
	if len(platforms) != 2 || platforms[0].ID != "intune" || platforms[0].Label != "Microsoft Intune" {
		t.Errorf("platforms not carried through: %+v", platforms)
	}
	configurations := manifest.Configurations
	if len(configurations) != 3 || configurations[0].SuggestedName != "Windows Fleet" {
		t.Errorf("configurations not carried through: %+v", configurations)
	}
}

func TestValidateTemplatesCatchesMissingRenderInput(t *testing.T) {
	dir := writeAssets(t, SchemaVersion)
	mustWrite(t, filepath.Join(dir, "windows", "intune", "EXTRA.txt.tmpl"), "missing={{.notInSchema}}")
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := strings.Replace(string(raw), `"windows/intune/obot-sentry.intunewin",
        "windows/intune/INSTRUCTIONS.md.tmpl"`, `"windows/intune/obot-sentry.intunewin",
        "windows/intune/INSTRUCTIONS.md.tmpl",
        "windows/intune/EXTRA.txt.tmpl"`, 1)
	mustWrite(t, filepath.Join(dir, "manifest.json"), manifest)
	loader, err := NewFS(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := loader.Find("intune", "windows")
	if err != nil {
		t.Fatal(err)
	}
	if err := loader.ValidateTemplates(configuration, completedValues(t, loader)); err == nil {
		t.Fatal("missing template input was not detected before download")
	}
}

func TestFind(t *testing.T) {
	l, err := NewFS(os.DirFS(writeAssets(t, SchemaVersion)))
	if err != nil {
		t.Fatal(err)
	}

	// Sole-OS platforms resolve without an OS; explicit picks work; the
	// rest error with the available list.
	if c, err := l.Find("intune", ""); err != nil || c.OS != "windows" {
		t.Errorf("Find(intune, \"\") = %+v, %v; want windows", c, err)
	}
	if c, err := l.Find("multi", "macos"); err != nil || c.OS != "macos" {
		t.Errorf("Find(multi, macos) = %+v, %v; want macos", c, err)
	}
	if _, err := l.Find("multi", ""); err == nil {
		t.Error("multi-OS platform without an OS should error")
	}
	if _, err := l.Find("android", ""); err == nil || !strings.Contains(err.Error(), "intune/windows") {
		t.Errorf("unknown platform should error naming the available configurations, got %v", err)
	}
}

// TestCompleteValues pins that fixed values, defaults, and bounds all
// come from the manifest's fields schema, not server code.
func TestCompleteValues(t *testing.T) {
	l, err := NewFS(os.DirFS(writeAssets(t, SchemaVersion)))
	if err != nil {
		t.Fatal(err)
	}

	// The caller supplies only serverURL; the default interval fills in.
	values := map[string]any{"serverURL": "https://obot.example.com"}
	if err := l.CompleteValues(values); err != nil {
		t.Fatalf("valid values rejected: %v", err)
	}
	if values["scanIntervalMinutes"] != int64(60) && values["scanIntervalMinutes"] != float64(60) && values["scanIntervalMinutes"] != 60 {
		t.Errorf("default scanIntervalMinutes not applied, got %v (%T)", values["scanIntervalMinutes"], values["scanIntervalMinutes"])
	}

	// Null means unset: the schema default applies instead of a type error.
	nulled := map[string]any{"serverURL": "https://obot.example.com", "scanIntervalMinutes": nil}
	if err := l.CompleteValues(nulled); err != nil {
		t.Errorf("null scanIntervalMinutes should fall back to the default, got %v", err)
	}
	if nulled["scanIntervalMinutes"] != int64(60) && nulled["scanIntervalMinutes"] != float64(60) && nulled["scanIntervalMinutes"] != 60 {
		t.Errorf("default not applied over null scanIntervalMinutes, got %v (%T)", nulled["scanIntervalMinutes"], nulled["scanIntervalMinutes"])
	}

	// obotSentryVersion is not a field: additionalProperties rejects it as
	// input (it enters render contexts from the manifest top level).
	spoofed := map[string]any{"serverURL": "https://obot.example.com", "obotSentryVersion": "9.9.9"}
	if err := l.CompleteValues(spoofed); err == nil {
		t.Error("caller-supplied obotSentryVersion should be rejected by the fields schema")
	}

	belowFloor := map[string]any{"serverURL": "https://obot.example.com", "scanIntervalMinutes": 5}
	if err := l.CompleteValues(belowFloor); err == nil {
		t.Error("scanIntervalMinutes below the schema minimum should be rejected")
	}

	if err := l.CompleteValues(map[string]any{}); err == nil {
		t.Error("missing required serverURL should be rejected")
	}
}

func completedValues(t *testing.T, l *Loader) map[string]any {
	t.Helper()
	values := map[string]any{"serverURL": "https://obot.example.com", "scanIntervalMinutes": 30}
	if err := l.CompleteValues(values); err != nil {
		t.Fatal(err)
	}
	return values
}

// TestRenderInstructions pins the UI setup guide: the configuration's
// markdown template rendered with the completed values.
func TestRenderInstructions(t *testing.T) {
	l, err := NewFS(os.DirFS(writeAssets(t, SchemaVersion)))
	if err != nil {
		t.Fatal(err)
	}
	c, err := l.Find("intune", "")
	if err != nil {
		t.Fatal(err)
	}
	instructions, err := l.RenderInstructions(c, completedValues(t, l))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(instructions, "# Setup") ||
		!strings.Contains(instructions, "server=https://obot.example.com") ||
		!strings.Contains(instructions, "interval=30") ||
		!strings.Contains(instructions, "version=1.2.3") {
		t.Errorf("instructions not rendered: %q", instructions)
	}
}

// TestZip pins the download contents: the installer verbatim and the
// template rendered with the values, both under base names with the
// .tmpl suffix stripped.
func TestZip(t *testing.T) {
	l, err := NewFS(os.DirFS(writeAssets(t, SchemaVersion)))
	if err != nil {
		t.Fatal(err)
	}
	c, err := l.Find("intune", "")
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := l.Zip(&buf, c, completedValues(t, l)); err != nil {
		t.Fatalf("zip: %v", err)
	}

	got := zipEntries(t, buf.Bytes())
	if len(got) != 2 {
		t.Fatalf("entries = %v, want installer + INSTRUCTIONS.md", keys(got))
	}
	if got["obot-sentry.intunewin"] != "fake-intunewin" {
		t.Errorf("installer not copied verbatim: %q", got["obot-sentry.intunewin"])
	}
	if !strings.Contains(got["INSTRUCTIONS.md"], "interval=30") {
		t.Errorf("INSTRUCTIONS.md not rendered: %q", got["INSTRUCTIONS.md"])
	}
}

// A template referencing a field outside the render context must fail
// the download rather than ship broken output (missingkey=error).
func TestZipRejectsUnknownTemplateField(t *testing.T) {
	dir := writeAssets(t, SchemaVersion)
	mustWrite(t, filepath.Join(dir, "windows", "intune", "INSTRUCTIONS.md.tmpl"), "oops={{.unknownField}}\n")
	l, err := NewFS(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	c, err := l.Find("intune", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Zip(&bytes.Buffer{}, c, completedValues(t, l)); err == nil {
		t.Error("template referencing an unknown field should error")
	}
}

func zipEntries(t *testing.T, b []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(rc)
		_ = rc.Close()
		out[f.Name] = string(data)
	}
	return out
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
