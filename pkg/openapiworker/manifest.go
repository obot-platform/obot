package openapiworker

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/obot-platform/obot/apiclient/types"
)

const (
	manifestVersion          = 1
	compilerSemanticsVersion = 1
	pathMatcherVersion       = 1
	parameterBindingVersion  = 1
)

type CanonicalOperationManifest struct {
	ManifestVersion          int         `json:"manifestVersion"`
	SchemaDigest             string      `json:"schemaDigest"`
	BaseURL                  string      `json:"baseURL"`
	ProtocolVersion          int         `json:"protocolVersion"`
	CompilerSemanticsVersion int         `json:"compilerSemanticsVersion"`
	PathMatcherVersion       int         `json:"pathMatcherVersion"`
	ParameterBindingVersion  int         `json:"parameterBindingVersion"`
	Operations               []Operation `json:"operations"`
}

type Operation struct {
	Method            string          `json:"method"`
	PathTemplate      string          `json:"pathTemplate"`
	OperationKey      string          `json:"operationKey"`
	OperationID       string          `json:"operationId,omitempty"`
	LegacyToolName    string          `json:"legacyToolName"`
	DisplayName       string          `json:"displayName,omitempty"`
	Description       string          `json:"description,omitempty"`
	Parameters        []Parameter     `json:"parameters,omitempty"`
	RequestBody       *RequestBody    `json:"requestBody,omitempty"`
	InputSchema       json.RawMessage `json:"inputSchema,omitempty"`
	RequestMediaTypes []string        `json:"requestMediaTypes,omitempty"`
}

type Parameter struct {
	Name     string          `json:"name"`
	In       string          `json:"in"`
	Required bool            `json:"required,omitempty"`
	Style    string          `json:"style,omitempty"`
	Explode  *bool           `json:"explode,omitempty"`
	Schema   json.RawMessage `json:"schema,omitempty"`
}

type RequestBody struct {
	Required   bool            `json:"required,omitempty"`
	MediaTypes map[string]bool `json:"mediaTypes,omitempty"`
}

type HeaderPolicy struct {
	Name        string `json:"name"`
	In          string `json:"in"`
	Required    bool   `json:"required,omitempty"`
	Prefix      string `json:"prefix,omitempty"`
	Sensitive   bool   `json:"sensitive,omitempty"`
	Forwardable bool   `json:"forwardable,omitempty"`
	Credential  bool   `json:"credential,omitempty"`
}

type HeaderPolicyDefinition struct {
	Name      string `json:"name"`
	Required  bool   `json:"required,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
	Sensitive bool   `json:"sensitive,omitempty"`
}

type HeaderPolicyDocument struct {
	Forwardable []HeaderPolicyDefinition `json:"forwardable"`
	Credentials []HeaderPolicyDefinition `json:"credentials"`
}

func (d HeaderPolicyDocument) Clone() HeaderPolicyDocument {
	return HeaderPolicyDocument{
		Forwardable: append([]HeaderPolicyDefinition(nil), d.Forwardable...),
		Credentials: append([]HeaderPolicyDefinition(nil), d.Credentials...),
	}
}

func (o Operation) Key() string {
	return OperationKey(o.Method, o.PathTemplate)
}

func OperationKey(method, pathTemplate string) string {
	return strings.ToUpper(method) + " " + pathTemplate
}

func SchemaDigest(schema []byte) string {
	sum := sha256.Sum256(schema)
	return hex.EncodeToString(sum[:])
}

func MarshalCanonical(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(value)
	if err != nil {
		return nil, fmt.Errorf("encode canonical JSON: %w", err)
	}
	encoded := bytes.TrimSuffix(buffer.Bytes(), []byte("\n"))
	if !json.Valid(encoded) {
		return nil, NewError(ErrorCodeProtocolError, "canonical JSON is invalid")
	}
	return encoded, nil
}

func ExecutionDigestV1(manifestBytes, policyBytes []byte) string {
	hash := sha256.New()
	writeLengthPrefixed(hash, []byte(digestDomain))
	writeLengthPrefixed(hash, manifestBytes)
	writeLengthPrefixed(hash, policyBytes)
	return hex.EncodeToString(hash.Sum(nil))
}

func writeLengthPrefixed(hash interface{ Write([]byte) (int, error) }, value []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = hash.Write(length[:])
	_, _ = hash.Write(value)
}

func BuildManifest(schema []byte, config types.OpenAPIRuntimeConfig, headers []types.MCPConfig) (CanonicalOperationManifest, []byte, []byte, string, error) {
	if len(schema) > MaxSchemaBytes {
		return CanonicalOperationManifest{}, nil, nil, "", NewError(ErrorCodeRequestTooLarge, "OpenAPI schema exceeds 1 MiB")
	}
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	document, err := loader.LoadFromData(schema)
	if err != nil {
		return CanonicalOperationManifest{}, nil, nil, "", NewError(ErrorCodeProtocolError, "cannot parse OpenAPI snapshot")
	}
	baseURL := strings.TrimSuffix(config.BaseURL, "/")
	if baseURL == "" {
		for _, server := range document.Servers {
			if server != nil && strings.HasPrefix(server.URL, "http") {
				baseURL = strings.TrimSuffix(server.URL, "/")
				break
			}
		}
	}
	if baseURL == "" {
		return CanonicalOperationManifest{}, nil, nil, "", NewError(ErrorCodeInvalidRequest, "OpenAPI execution requires a base URL")
	}

	manifest := CanonicalOperationManifest{
		ManifestVersion:          manifestVersion,
		SchemaDigest:             SchemaDigest(schema),
		BaseURL:                  baseURL,
		ProtocolVersion:          ProtocolVersion,
		CompilerSemanticsVersion: compilerSemanticsVersion,
		PathMatcherVersion:       pathMatcherVersion,
		ParameterBindingVersion:  parameterBindingVersion,
	}
	pathNames := make([]string, 0, len(document.Paths.Map()))
	for path := range document.Paths.Map() {
		pathNames = append(pathNames, path)
	}
	sort.Strings(pathNames)
	for _, path := range pathNames {
		item := document.Paths.Map()[path]
		if item == nil {
			continue
		}
		for method, operation := range item.Operations() {
			if operation == nil {
				continue
			}
			manifest.Operations = append(manifest.Operations, buildOperation(method, path, item, operation))
		}
	}
	if err := validateUniqueOperations(manifest.Operations); err != nil {
		return CanonicalOperationManifest{}, nil, nil, "", err
	}
	manifestBytes, err := MarshalCanonical(manifest)
	if err != nil {
		return CanonicalOperationManifest{}, nil, nil, "", err
	}
	policyDocument := HeaderPolicyDocumentFromManifest(manifest, headers)
	policyBytes, err := MarshalCanonical(policyDocument)
	if err != nil {
		return CanonicalOperationManifest{}, nil, nil, "", err
	}
	return manifest, manifestBytes, policyBytes, ExecutionDigestV1(manifestBytes, policyBytes), nil
}

func HeaderPolicyDocumentFromManifest(manifest CanonicalOperationManifest, headers []types.MCPConfig) HeaderPolicyDocument {
	document := HeaderPolicyDocument{
		Forwardable: []HeaderPolicyDefinition{},
		Credentials: []HeaderPolicyDefinition{},
	}
	seenForwardable := make(map[string]struct{})
	for _, operation := range manifest.Operations {
		for _, parameter := range operation.Parameters {
			if parameter.In != "header" || IsProtectedHeader(parameter.Name) {
				continue
			}
			canonicalName := strings.ToLower(parameter.Name)
			if _, exists := seenForwardable[canonicalName]; exists {
				continue
			}
			seenForwardable[canonicalName] = struct{}{}
			document.Forwardable = append(document.Forwardable, HeaderPolicyDefinition{Name: parameter.Name, Required: parameter.Required})
		}
	}
	for _, header := range headers {
		if header.Usage != types.Header || !header.UserAllowed {
			continue
		}
		document.Credentials = append(document.Credentials, HeaderPolicyDefinition{
			Name:      header.Name,
			Required:  header.Required,
			Prefix:    header.Prefix,
			Sensitive: true,
		})
	}
	return document
}

func buildOperation(method, path string, item *openapi3.PathItem, operation *openapi3.Operation) Operation {
	result := Operation{
		Method:         strings.ToUpper(method),
		PathTemplate:   path,
		OperationKey:   OperationKey(method, path),
		OperationID:    operation.OperationID,
		LegacyToolName: legacyToolName(operation.OperationID, method, path),
		DisplayName:    operation.Summary,
		Description:    operation.Description,
		InputSchema:    operationInputSchemaFromParameters(operation.Parameters),
	}
	if result.DisplayName == "" {
		result.DisplayName = result.OperationKey
	}
	for _, ref := range append(item.Parameters, operation.Parameters...) {
		if ref == nil || ref.Value == nil {
			continue
		}
		parameter := Parameter{Name: ref.Value.Name, In: ref.Value.In, Required: ref.Value.Required, Style: ref.Value.Style, Explode: ref.Value.Explode}
		if ref.Value.Schema != nil && ref.Value.Schema.Value != nil {
			parameter.Schema, _ = MarshalCanonical(ref.Value.Schema.Value)
		}
		result.Parameters = append(result.Parameters, parameter)
	}
	if operation.RequestBody != nil && operation.RequestBody.Value != nil {
		body := &RequestBody{Required: operation.RequestBody.Value.Required, MediaTypes: map[string]bool{}}
		for mediaType := range operation.RequestBody.Value.Content {
			body.MediaTypes[mediaType] = true
		}
		result.RequestBody = body
		mediaTypes := make([]string, 0, len(operation.RequestBody.Value.Content))
		for mediaType := range operation.RequestBody.Value.Content {
			mediaTypes = append(mediaTypes, mediaType)
		}
		sort.Strings(mediaTypes)
		result.RequestMediaTypes = mediaTypes
	}
	return result
}

func operationInputSchemaFromParameters(parameters []*openapi3.ParameterRef) json.RawMessage {
	type property struct {
		Type string `json:"type"`
	}
	schema := struct {
		Type       string              `json:"type"`
		Properties map[string]property `json:"properties,omitempty"`
		Required   []string            `json:"required,omitempty"`
	}{
		Type:       "object",
		Properties: map[string]property{},
	}
	for _, ref := range parameters {
		if ref == nil || ref.Value == nil {
			continue
		}
		parameterType := "string"
		if ref.Value.Schema != nil && ref.Value.Schema.Value != nil && ref.Value.Schema.Value.Type != nil {
			parameterType = ref.Value.Schema.Value.Type.Slice()[0]
		}
		schema.Properties[ref.Value.Name] = property{Type: parameterType}
		if ref.Value.Required {
			schema.Required = append(schema.Required, ref.Value.Name)
		}
	}
	encoded, _ := MarshalCanonical(schema)
	return encoded
}

func legacyToolName(operationID, method, path string) string {
	if operationID != "" {
		return operationID
	}
	replacer := strings.NewReplacer("/", "_", "{", "", "}", "", "-", "_", ".", "_")
	name := replacer.Replace(strings.ToLower(strings.Trim(path, "/")))
	if name == "" {
		name = "root"
	}
	return strings.ToLower(method) + "_" + name
}

func validateUniqueOperations(operations []Operation) error {
	seen := make(map[string]struct{}, len(operations))
	toolNames := make(map[string]struct{}, len(operations))
	for _, operation := range operations {
		if _, exists := seen[operation.OperationKey]; exists {
			return NewError(ErrorCodeProtocolError, "OpenAPI contains duplicate method and path template")
		}
		seen[operation.OperationKey] = struct{}{}
		if _, exists := toolNames[operation.LegacyToolName]; exists {
			return NewError(ErrorCodeProtocolError, "OpenAPI contains duplicate legacy tool names")
		}
		toolNames[operation.LegacyToolName] = struct{}{}
	}
	return nil
}
