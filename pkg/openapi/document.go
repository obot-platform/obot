package openapi

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/obot-platform/obot/apiclient/types"
)

var (
	// Match the versions accepted by mcp-images/openapi-mcp/config.py. Kin-openapi
	// supports more versions, but accepting those here would defer failure until
	// container startup. Expand this range together with wrapper support.
	versionPattern = regexp.MustCompile(`^3\.(0\.[0-4]|1\.[0-2])$`)

	methods = map[string]bool{
		"GET": true, "PUT": true, "POST": true, "DELETE": true,
		"OPTIONS": true, "HEAD": true, "PATCH": true, "TRACE": true,
	}
)

// inspect loads a typed OpenAPI document to resolve the API destination and
// suggest credential headers, checking the hosted wrapper's supported subset.
// The original normalized bytes are retained separately for snapshots; loading
// and resolving references must not rewrite the stored document.
func inspect(raw map[string]any, canonical []byte, config types.OpenAPIRuntimeConfig) (*Result, error) {
	// The wrapper checks references even in extensions and examples. Keep this
	// guard in addition to the loader's prohibition on external references.
	if err := walkReferences(raw, raw); err != nil {
		return nil, err
	}

	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	document, err := loader.LoadFromData(canonical)
	if err != nil {
		// Loader errors may quote schema content. Do not expose embedded secrets.
		return nil, fmt.Errorf("cannot parse OpenAPI document or resolve local references")
	}

	if !versionPattern.MatchString(document.OpenAPI) {
		return nil, fmt.Errorf("supported OpenAPI versions are 3.0.0–3.0.4 and 3.1.0–3.1.2")
	}
	if document.Info == nil || strings.TrimSpace(document.Info.Title) == "" || strings.TrimSpace(document.Info.Version) == "" {
		return nil, fmt.Errorf("schema requires info.title and info.version")
	}
	if len(document.Webhooks) > 0 {
		return nil, fmt.Errorf("OpenAPI webhooks are unsupported")
	}

	baseURL, err := findBaseURL(config.BaseURL, document.Servers)
	if err != nil {
		return nil, err
	}
	result := &Result{BaseURL: baseURL}

	headers, err := securityHeaders(document)
	if err != nil {
		return nil, err
	}
	result.SuggestedHeaders = headers

	if document.Paths == nil {
		return nil, fmt.Errorf("schema paths must be an object")
	}
	for path, item := range document.Paths.Map() {
		if item == nil || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.ContainsAny(path, "\\\\?#") || strings.ContainsFunc(path, unicode.IsSpace) {
			return nil, fmt.Errorf("paths must be local path templates with object definitions")
		}
		if item.Ref != "" {
			return nil, fmt.Errorf("referenced path items are unsupported")
		}

		if err := checkParameters(item.Parameters); err != nil {
			return nil, err
		}

		for method, op := range item.Operations() {
			if !methods[method] {
				return nil, fmt.Errorf("operation uses an unsupported HTTP method")
			}
			if len(op.Callbacks) > 0 {
				return nil, fmt.Errorf("OpenAPI callbacks are unsupported")
			}
			if err := checkParameters(op.Parameters); err != nil {
				return nil, err
			}
		}
	}

	return result, nil
}

func findBaseURL(configured string, servers openapi3.Servers) (string, error) {
	if configured != "" {
		return destination(configured)
	}

	for _, server := range servers {
		if server == nil {
			continue
		}
		if base, err := destination(server.URL); err == nil {
			return base, nil
		}
	}

	return "", fmt.Errorf("no usable server URL; configure baseURL")
}

// securityHeaders turns declared API-key and bearer schemes into editable Obot
// header inputs. It deduplicates header names and suggests Obot's Bearer prefix;
// it neither obtains credentials nor adds any values to the schema.
// OAuth declarations remain in the snapshot but do not produce header inputs:
// the hosted runtime does not perform OAuth flows.
func securityHeaders(document *openapi3.T) ([]types.MCPConfig, error) {
	var headers []types.MCPConfig
	if document.Components == nil {
		return headers, nil
	}

	schemes := document.Components.SecuritySchemes
	seen := map[string]int{}

	// Stable order makes suggestions reproducible.
	keys := make([]string, 0, len(schemes))
	for key := range schemes {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		ref := schemes[key]
		if ref == nil || ref.Value == nil {
			return nil, fmt.Errorf("unresolved or cyclic security reference")
		}

		scheme := ref.Value
		var name, prefix string
		switch {
		case scheme.Type == "oauth2":
			continue
		case scheme.Type == "apiKey" && scheme.In == "header":
			name = scheme.Name
		case scheme.Type == "http" && strings.EqualFold(scheme.Scheme, "bearer"):
			name, prefix = "Authorization", "Bearer "
		default:
			return nil, fmt.Errorf("only header API keys and pre-issued bearer credentials are supported; OAuth declarations are ignored")
		}

		if err := validateHeader(name); err != nil {
			return nil, err
		}

		lower := strings.ToLower(name)
		if index, exists := seen[lower]; exists {
			if headers[index].Prefix != prefix {
				return nil, fmt.Errorf("security schemes disagree on a header's credential prefix")
			}
			continue
		}

		seen[lower] = len(headers)
		headers = append(headers, types.MCPConfig{
			Name:        name,
			Key:         name,
			Description: scheme.Description,
			Required:    true,
			Sensitive:   true,
			Prefix:      prefix,
			Usage:       types.Header,
		})
	}

	return headers, nil
}

func checkParameters(parameters openapi3.Parameters) error {
	for _, ref := range parameters {
		if ref == nil || ref.Value == nil {
			return fmt.Errorf("unresolved or cyclic parameter reference")
		}

		parameter := ref.Value
		if parameter.In == "cookie" {
			return fmt.Errorf("cookie parameters are unsupported")
		}
		if parameter.In == "header" {
			if err := validateHeader(parameter.Name); err != nil {
				return fmt.Errorf("transport headers cannot be tool parameters")
			}
		}
	}

	return nil
}

// reference checks that a $ref is a local JSON pointer to an existing node.
// It returns the target without expanding or modifying the document.
func reference(document map[string]any, value any) (any, error) {
	ref, ok := value.(string)
	if !ok || !strings.HasPrefix(ref, "#/") {
		return nil, fmt.Errorf("only local JSON pointer references are supported")
	}

	pointer, err := url.PathUnescape(ref[2:])
	if err != nil {
		return nil, fmt.Errorf("invalid local reference")
	}

	var node any = document
	for part := range strings.SplitSeq(pointer, "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch current := node.(type) {
		case map[string]any:
			var exists bool
			node, exists = current[part]
			if !exists {
				return nil, fmt.Errorf("unresolved local reference")
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(current) {
				return nil, fmt.Errorf("unresolved local reference")
			}
			node = current[index]
		default:
			return nil, fmt.Errorf("unresolved local reference")
		}
	}

	return node, nil
}

// walkReferences checks refs throughout the raw document, including extensions
// and examples that the typed OpenAPI loader may leave untouched. It rejects
// dynamic refs and invalid local pointers without following refs recursively.
func walkReferences(document map[string]any, node any) error {
	switch node := node.(type) {
	case map[string]any:
		for key, value := range node {
			switch key {
			case "$dynamicRef", "$recursiveRef":
				return fmt.Errorf("dynamic and recursive JSON Schema references are unsupported")
			case "$ref":
				if _, err := reference(document, value); err != nil {
					return err
				}
			}

			if err := walkReferences(document, value); err != nil {
				return err
			}
		}
	case []any:
		for _, value := range node {
			if err := walkReferences(document, value); err != nil {
				return err
			}
		}
	}

	return nil
}
