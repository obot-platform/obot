// Package mdmassets loads the MDM assets tree that obot-sentry's build
// publishes and assembles the
// per-configuration ZIP an admin downloads. The manifest is the whole
// contract: it declares the platforms (identity + display info), the
// configurations (per platform+OS, with their rendered-markdown
// instructions template and asset files), and the field schema the
// admin form renders from — obot holds no platform-specific knowledge
// of its own.
//
// A source directory or tarball contains manifest.json plus its referenced
// files. The controller imports that source into immutable database bundles;
// HTTP replicas open those bundles through this package and never serve from
// the source path. The format is defined by these manifest types and produced
// by obot-sentry's build/mdm-assets.sh.
package mdmassets

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"
	"text/template"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/obot-platform/obot/apiclient/types"
)

// SchemaVersion is the manifest format this loader understands. An
// assets tree declaring anything else is rejected so a newer format
// can't be misread by an older server.
const SchemaVersion = "v1"

var artifactSlugNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// Loader validates and renders files from one immutable MDM asset bundle.
type Loader struct {
	files    fs.FS
	manifest types.MDMAssetManifest
	fields   *jsonschema.Resolved
}

// NewFS loads and validates an assets snapshot rooted at files. Callers must
// not mutate files after this function returns.
func NewFS(files fs.FS) (*Loader, error) {
	raw, err := fs.ReadFile(files, "manifest.json")
	if err != nil {
		return nil, fmt.Errorf("reading MDM assets manifest: %w", err)
	}
	var m types.MDMAssetManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parsing MDM assets manifest: %w", err)
	}
	if m.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("MDM assets manifest schemaVersion %q is unsupported (want %q)", m.SchemaVersion, SchemaVersion)
	}
	if strings.TrimSpace(m.ObotSentryVersion) == "" || strings.TrimSpace(m.ObotSentryVersion) != m.ObotSentryVersion {
		return nil, fmt.Errorf("MDM assets manifest obotSentryVersion must be non-empty and have no surrounding whitespace")
	}

	var fieldsSchema jsonschema.Schema
	if err := json.Unmarshal(m.Fields, &fieldsSchema); err != nil {
		return nil, fmt.Errorf("parsing MDM assets manifest fields schema: %w", err)
	}
	fields, err := fieldsSchema.Resolve(&jsonschema.ResolveOptions{ValidateDefaults: true})
	if err != nil {
		return nil, fmt.Errorf("resolving MDM assets manifest fields schema: %w", err)
	}

	// Fail at startup, not at download time, on structural problems:
	// duplicate ids/units, dangling platform references, instructions
	// not shipped in the download, or missing files.
	platformIDs := map[string]bool{}
	for _, p := range m.Platforms {
		if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.ID) != p.ID {
			return nil, fmt.Errorf("MDM assets manifest platform ids must be non-empty and have no surrounding whitespace")
		}
		if platformIDs[p.ID] {
			return nil, fmt.Errorf("MDM assets manifest declares platform %s twice", p.ID)
		}
		platformIDs[p.ID] = true
		if p.Icon != "" {
			if err := validateFile(files, p.Icon); err != nil {
				return nil, fmt.Errorf("MDM assets manifest platform %s references a missing icon: %w", p.ID, err)
			}
		}
	}
	units := map[string]bool{}
	slugs := map[string]string{}
	for _, c := range m.Configurations {
		if strings.TrimSpace(c.Platform) == "" || strings.TrimSpace(c.OS) == "" ||
			strings.TrimSpace(c.Platform) != c.Platform || strings.TrimSpace(c.OS) != c.OS {
			return nil, fmt.Errorf("MDM assets manifest configurations require non-empty platform and os without surrounding whitespace")
		}
		unit := c.Platform + "/" + c.OS
		if !platformIDs[c.Platform] {
			return nil, fmt.Errorf("MDM assets manifest configuration %s references undeclared platform %s", unit, c.Platform)
		}
		if units[unit] {
			return nil, fmt.Errorf("MDM assets manifest declares configuration %s twice", unit)
		}
		units[unit] = true
		slug := ArtifactSlug(c.Platform, c.OS)
		if prior, ok := slugs[slug]; ok {
			return nil, fmt.Errorf("MDM assets manifest configurations %s and %s produce the same download slug %q", prior, unit, slug)
		}
		slugs[slug] = unit
		if !slices.Contains(c.Assets, c.Instructions) {
			return nil, fmt.Errorf("MDM assets manifest configuration %s does not list its instructions template in assets", unit)
		}
		assetNames := map[string]struct{}{}
		outputNames := map[string]string{}
		for _, rel := range c.Assets {
			if _, exists := assetNames[rel]; exists {
				return nil, fmt.Errorf("MDM assets manifest configuration %s lists asset %q twice", unit, rel)
			}
			assetNames[rel] = struct{}{}
			if err := validateFile(files, rel); err != nil {
				return nil, fmt.Errorf("MDM assets manifest configuration %s references a missing file: %w", unit, err)
			}
			outputName := strings.TrimSuffix(path.Base(rel), ".tmpl")
			if prior, exists := outputNames[outputName]; exists {
				return nil, fmt.Errorf("MDM assets manifest configuration %s assets %q and %q both produce ZIP file %q", unit, prior, rel, outputName)
			}
			outputNames[outputName] = rel
			if rel == c.Instructions || strings.HasSuffix(path.Base(rel), ".tmpl") {
				if _, err := template.New(path.Base(rel)).Option("missingkey=error").ParseFS(files, rel); err != nil {
					return nil, fmt.Errorf("MDM assets manifest configuration %s contains invalid template %q: %w", unit, rel, err)
				}
			}
		}
	}
	return &Loader{files: files, manifest: m, fields: fields}, nil
}

// ArtifactSlug returns the stable URL segment for a platform/OS artifact.
// NewFS rejects manifests whose targets collide after normalization.
func ArtifactSlug(platform, osName string) string {
	slug := artifactSlugNonAlnum.ReplaceAllString(strings.ToLower(platform+"-"+osName), "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "configuration"
	}
	return slug
}

func validateFile(files fs.FS, name string) error {
	if name == "" || !fs.ValidPath(name) || strings.Contains(name, `\`) {
		return fmt.Errorf("invalid relative asset path %q", name)
	}
	info, err := fs.Stat(files, name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("asset path %q is not a regular file", name)
	}
	return nil
}

// Manifest returns a detached copy of the validated manifest for persistence
// and API discovery without reopening the archive.
func (l *Loader) Manifest() types.MDMAssetManifest {
	configurations := slices.Clone(l.manifest.Configurations)
	for i := range configurations {
		configurations[i].Assets = slices.Clone(configurations[i].Assets)
	}
	return types.MDMAssetManifest{
		SchemaVersion:     l.manifest.SchemaVersion,
		ObotSentryVersion: l.manifest.ObotSentryVersion,
		Fields:            bytes.Clone(l.manifest.Fields),
		Platforms:         slices.Clone(l.manifest.Platforms),
		Configurations:    configurations,
	}
}

// Find returns the configuration for (platform, osName). An empty
// osName matches when the platform has exactly one configuration. The
// returned error names what is available and is safe to surface to the
// admin.
func (l *Loader) Find(platform, osName string) (types.MDMAssetConfiguration, error) {
	var matches []types.MDMAssetConfiguration
	for _, c := range l.manifest.Configurations {
		if c.Platform == platform && (osName == "" || c.OS == osName) {
			matches = append(matches, c)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		var available []string
		for _, c := range l.manifest.Configurations {
			available = append(available, c.Platform+"/"+c.OS)
		}
		return types.MDMAssetConfiguration{}, fmt.Errorf("the MDM asset bundle has no %s configuration (available: %s)",
			strings.TrimSuffix(platform+"/"+osName, "/"), strings.Join(available, ", "))
	default:
		return types.MDMAssetConfiguration{}, fmt.Errorf("platform %s targets multiple OSes; specify one", platform)
	}
}

// CompleteValues drops nulls (null means unset), fills schema defaults,
// and validates values in place — every rule comes from the manifest.
// The returned error is safe to surface to the admin.
func (l *Loader) CompleteValues(values map[string]any) error {
	for name, value := range values {
		if value == nil {
			delete(values, name)
		}
	}
	if err := l.fields.ApplyDefaults(&values); err != nil {
		return err
	}
	return l.fields.Validate(values)
}

// renderContext is the validated values plus the loader-supplied
// obotSentryVersion — added here rather than as a field because it is not
// admin input and the fields schema (additionalProperties: false)
// rejects unknown value keys.
func (l *Loader) renderContext(values map[string]any) map[string]any {
	context := make(map[string]any, len(values)+1)
	maps.Copy(context, values)
	context["obotSentryVersion"] = l.manifest.ObotSentryVersion
	return context
}

// RenderInstructions renders the configuration's instructions template
// with values into the markdown setup guide clients display.
func (l *Loader) RenderInstructions(c types.MDMAssetConfiguration, values map[string]any) (string, error) {
	var buf bytes.Buffer
	if err := l.renderTemplate(&buf, c.Instructions, l.renderContext(values)); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// ValidateTemplates executes every rendered asset against completed values
// without copying binary assets. This catches missing template inputs before a
// deployment is saved instead of deferring the failure until download.
func (l *Loader) ValidateTemplates(c types.MDMAssetConfiguration, values map[string]any) error {
	context := l.renderContext(values)
	for _, rel := range c.Assets {
		if !strings.HasSuffix(path.Base(rel), ".tmpl") {
			continue
		}
		if err := l.renderTemplate(io.Discard, rel, context); err != nil {
			return err
		}
	}
	return nil
}

// RenderedArtifact contains one fully rendered platform/OS download. Callers
// persist Content privately and expose only the platform, OS, and instructions.
type RenderedArtifact struct {
	Platform     string
	OS           string
	Instructions string
	Content      []byte
}

// RenderAll completes and validates values once, then renders every target in
// the immutable bundle. It returns no partial result if any target fails.
func (l *Loader) RenderAll(values map[string]any) ([]RenderedArtifact, error) {
	completed := make(map[string]any, len(values))
	maps.Copy(completed, values)
	if err := l.CompleteValues(completed); err != nil {
		return nil, fmt.Errorf("invalid configuration values: %w", err)
	}

	artifacts := make([]RenderedArtifact, 0, len(l.manifest.Configurations))
	for _, configuration := range l.manifest.Configurations {
		instructions, err := l.RenderInstructions(configuration, completed)
		if err != nil {
			return nil, err
		}
		var content bytes.Buffer
		if err := l.Zip(&content, configuration, completed); err != nil {
			return nil, err
		}
		artifacts = append(artifacts, RenderedArtifact{
			Platform:     configuration.Platform,
			OS:           configuration.OS,
			Instructions: instructions,
			Content:      content.Bytes(),
		})
	}
	return artifacts, nil
}

// Zip writes the configuration's download to w: assets ending in .tmpl
// are rendered as Go text/templates against values (suffix stripped),
// everything else is copied verbatim. Names in the ZIP are base names
// — the assemble step guarantees they don't collide.
func (l *Loader) Zip(w io.Writer, c types.MDMAssetConfiguration, values map[string]any) error {
	zw := zip.NewWriter(w)
	context := l.renderContext(values)
	for _, rel := range c.Assets {
		name := path.Base(rel)
		var err error
		if strings.HasSuffix(name, ".tmpl") {
			err = renderEntry(zw, l, rel, strings.TrimSuffix(name, ".tmpl"), context)
		} else {
			err = copyInto(zw, l.files, rel, name)
		}
		if err != nil {
			_ = zw.Close()
			return err
		}
	}
	return zw.Close()
}

// renderTemplate renders the template asset at rel with values into w.
// missingkey=error keeps a template/fields mismatch from shipping a
// broken download silently.
func (l *Loader) renderTemplate(w io.Writer, rel string, values map[string]any) error {
	t, err := template.New(path.Base(rel)).Option("missingkey=error").ParseFS(l.files, rel)
	if err != nil {
		return fmt.Errorf("parsing template %s: %w", rel, err)
	}
	if err := t.ExecuteTemplate(w, path.Base(rel), values); err != nil {
		return fmt.Errorf("rendering template %s: %w", rel, err)
	}
	return nil
}

// renderEntry renders the template asset at rel into the zip under name.
func renderEntry(zw *zip.Writer, l *Loader, rel, name string, values map[string]any) error {
	var buf bytes.Buffer
	if err := l.renderTemplate(&buf, rel, values); err != nil {
		return err
	}
	f, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = f.Write(buf.Bytes())
	return err
}

// copyInto streams the file at rel into the zip under name.
func copyInto(zw *zip.Writer, files fs.FS, rel, name string) error {
	in, err := files.Open(rel)
	if err != nil {
		return fmt.Errorf("reading assets file %s: %w", name, err)
	}
	defer func() { _ = in.Close() }()
	f, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, in)
	return err
}
