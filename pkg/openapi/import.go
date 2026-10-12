// Package openapi imports immutable OpenAPI snapshots for the hosted wrapper.
// It does not generate MCP tools or make requests to the described API.
package openapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/safehttp"
	yamlv3 "go.yaml.in/yaml/v3"
	"sigs.k8s.io/yaml"
)

const (
	MaxSchemaBytes = 1024 * 1024
)

// Importer fetches schema sources. It never receives API credentials.
type Importer struct {
	client  *http.Client
	options safehttp.Options
	devMode bool
}

// Result contains the stored schema, resolved API destination, and suggested
// credential inputs. MCP tools are generated and listed by the hosted wrapper.
type Result struct {
	Schema           json.RawMessage
	BaseURL          string
	SuggestedHeaders []types.MCPConfig
}

// NewImporter applies the configured network policy to schema fetches. devMode
// permits HTTP schema sources and API destinations for local development.
// Redirects are not followed, including same-origin ones.
func NewImporter(options safehttp.Options, devMode bool) *Importer {
	// Schema sources never receive credentials configured for API requests.
	options.Headers = nil
	options.TokenSource = nil
	if options.Timeout == 0 {
		options.Timeout = 30 * time.Second
	}
	client := safehttp.NewClient(options)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Importer{client: client, options: options, devMode: devMode}
}

// Import always reads Source anew, ignoring any previous Schema snapshot.
// It returns no replacement snapshot on failure; callers retain the last good one.
func (i *Importer) Import(ctx context.Context, config types.OpenAPIRuntimeConfig) (*Result, error) {
	source := config.Source
	if (source.URL == "") == (source.Content == "") {
		return nil, fmt.Errorf("exactly one OpenAPI source URL or content is required")
	}

	if source.URL == "" {
		return i.parse(ctx, []byte(source.Content), config)
	}

	u, err := schemaSourceURL(source.URL, i.devMode)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("invalid schema request: %w", err)
	}

	resp, err := i.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("schema fetch failed (network policy, connection, or timeout)")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("schema fetch returned HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxSchemaBytes+1))
	if err != nil {
		return nil, fmt.Errorf("cannot read schema response")
	}
	if len(data) > MaxSchemaBytes {
		return nil, fmt.Errorf("schema exceeds 1 MiB")
	}

	return i.parse(ctx, data, config)
}

// parse checks the resolved destination against the importer's transport and
// network policies.
// It makes no request to the destination.
func (i *Importer) parse(ctx context.Context, data []byte, config types.OpenAPIRuntimeConfig) (*Result, error) {
	result, err := Parse(data, config)
	if err != nil {
		return nil, err
	}
	if !i.devMode && strings.HasPrefix(result.BaseURL, "http://") {
		return nil, fmt.Errorf("API destination must use HTTPS unless Obot development mode is enabled")
	}
	if err := safehttp.ValidateURL(ctx, result.BaseURL, i.options); err != nil {
		return nil, fmt.Errorf("API destination is blocked: %w", err)
	}
	return result, nil
}

// Parse normalizes JSON/YAML and checks the wrapper's supported subset. It does
// not resolve remote references or claim to validate every OpenAPI constraint.
// The importer applies transport and network policies after parsing.
// SuggestedHeaders are editable definitions, not credential values.
func Parse(data []byte, config types.OpenAPIRuntimeConfig) (*Result, error) {
	document, canonical, err := normalize(data)
	if err != nil {
		return nil, err
	}

	result, err := inspect(document, canonical, config)
	if err != nil {
		return nil, err
	}

	result.Schema = canonical
	return result, nil
}

// normalize accepts one bounded JSON/YAML object and produces compact JSON with
// sorted object keys. This removes formatting and key-order differences from
// stored snapshots without expanding references or rewriting the API document.
// It also returns the decoded object for wrapper-specific reference checks.
func normalize(data []byte) (map[string]any, []byte, error) {
	if len(data) > MaxSchemaBytes {
		return nil, nil, fmt.Errorf("schema exceeds 1 MiB")
	}
	if !utf8.Valid(data) {
		return nil, nil, fmt.Errorf("schema must be UTF-8")
	}

	// Inspect YAML nodes before conversion to reject duplicate keys, aliases,
	// multiple documents, and excessive nesting without expanding aliases.
	decoder := yamlv3.NewDecoder(bytes.NewReader(data))
	var node yamlv3.Node
	if err := decoder.Decode(&node); err != nil {
		return nil, nil, fmt.Errorf("schema must be a JSON or YAML document")
	}
	if err := checkYAML(&node, 0); err != nil {
		return nil, nil, err
	}

	var extra yamlv3.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, nil, fmt.Errorf("schema must contain exactly one document")
	}

	if !json.Valid(data) {
		var err error
		data, err = yaml.YAMLToJSONStrict(data)
		if err != nil {
			return nil, nil, fmt.Errorf("schema cannot be converted to JSON")
		}
	}

	var document map[string]any
	jsonDecoder := json.NewDecoder(bytes.NewReader(data))
	jsonDecoder.UseNumber()
	if err := jsonDecoder.Decode(&document); err != nil || document == nil {
		return nil, nil, fmt.Errorf("schema must be an object")
	}

	canonical, err := json.Marshal(document)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot normalize schema")
	}
	if len(canonical) > MaxSchemaBytes {
		return nil, nil, fmt.Errorf("normalized schema exceeds 1 MiB")
	}

	return document, canonical, nil
}

// checkYAML rejects excessive nesting, aliases, and invalid or duplicate mapping
// keys before conversion to JSON. The 128-level cap bounds recursive processing
// of untrusted input; it is an application policy, not an OpenAPI limit.
func checkYAML(node *yamlv3.Node, depth int) error {
	if depth > 128 {
		return fmt.Errorf("schema nesting exceeds 128 levels")
	}
	if node.Kind == yamlv3.AliasNode {
		return fmt.Errorf("YAML aliases are unsupported; inline the referenced content")
	}
	if node.Kind == yamlv3.MappingNode {
		seen := map[string]bool{}
		for n := 0; n < len(node.Content); n += 2 {
			key := node.Content[n]
			// YAML commonly uses numeric response codes as object keys.
			if key.Kind != yamlv3.ScalarNode || (key.Tag != "!!str" && key.Tag != "!!int") || seen[key.Value] {
				return fmt.Errorf("schema object keys must be unique strings")
			}
			seen[key.Value] = true
		}
	}
	for _, child := range node.Content {
		if err := checkYAML(child, depth+1); err != nil {
			return err
		}
	}
	return nil
}
