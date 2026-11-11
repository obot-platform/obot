package apiproxy

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	apiclienttypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type DocumentationHandler struct {
	Executor  Executor
	Storage   kclient.Client
	ServerURL string
}

type Documentation struct {
	VMCPID     string                   `json:"vmcpID"`
	Components []ComponentDocumentation `json:"components"`
}

type ComponentDocumentation struct {
	ID              string                   `json:"id"`
	DisplayName     string                   `json:"displayName"`
	BasePath        string                   `json:"basePath"`
	ExecutionDigest string                   `json:"executionDigest"`
	Operations      []OperationDocumentation `json:"operations"`
}

type OperationDocumentation struct {
	OperationKey         string                   `json:"operationKey"`
	LegacyToolName       string                   `json:"legacyToolName"`
	DisplayName          string                   `json:"displayName"`
	Description          string                   `json:"description,omitempty"`
	Method               string                   `json:"method"`
	PathTemplate         string                   `json:"pathTemplate"`
	Parameters           []ParameterDocumentation `json:"parameters,omitempty"`
	RequestMediaTypes    []string                 `json:"requestMediaTypes,omitempty"`
	BusinessAuthRequired bool                     `json:"businessAuthRequired"`
	Example              string                   `json:"example"`
}

type OpenAPIDocument struct {
	OpenAPI    string                    `json:"openapi"`
	Info       OpenAPIInfo               `json:"info"`
	Servers    []OpenAPIServer           `json:"servers"`
	Paths      map[string]map[string]any `json:"paths"`
	Components OpenAPIComponents         `json:"components"`
}

type OpenAPIInfo struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
}

type OpenAPIServer struct {
	URL string `json:"url"`
}

type OpenAPIComponents struct {
	SecuritySchemes map[string]OpenAPISecurityScheme `json:"securitySchemes,omitempty"`
}

type OpenAPISecurityScheme struct {
	Type string `json:"type"`
	Name string `json:"name"`
	In   string `json:"in"`
}

func (h *DocumentationHandler) Get(req api.Context) error {
	vmcpID := req.PathValue("vmcp_id")
	var vmcp v1.VMCP
	if err := req.Get(&vmcp, vmcpID); err != nil {
		return fmt.Errorf("failed to get VMCP: %w", err)
	}
	instance, err := vmcpconfig.FindInstance(req.Context(), h.Storage, vmcp.Namespace, vmcp.Name, req.User.GetUID())
	if err != nil || instance == nil {
		return apiclienttypes.NewErrNotFound("VMCP connection not found")
	}

	documentation := Documentation{VMCPID: vmcp.Name}
	for _, component := range vmcpconfig.ComponentsForInstance(vmcp, *instance) {
		if component.CatalogEntry.Manifest.Runtime != apiclienttypes.RuntimeOpenAPI {
			continue
		}
		description, describeErr := h.Executor.Describe(req.Context(), req.User, Reference{VMCPID: vmcp.Name, ComponentID: component.ID})
		if describeErr != nil {
			slog.Warn("OpenAPI Worker documentation is unavailable", "error", describeErr.Error())
			status := httpStatusForDocumentation(describeErr)
			return apiclienttypes.NewErrHTTP(status, "OpenAPI Worker documentation is unavailable")
		}
		basePath := strings.TrimSuffix(h.ServerURL, "/") + "/api-proxy/" + url.PathEscape(vmcp.Name) + "/" + url.PathEscape(component.ID)
		documentation.Components = append(documentation.Components, ComponentDocumentation{
			ID:              component.ID,
			DisplayName:     component.Name,
			BasePath:        basePath,
			ExecutionDigest: description.ExecutionDigest,
			Operations:      operations(basePath, description),
		})
	}
	if documentation.Components == nil {
		documentation.Components = []ComponentDocumentation{}
	}
	return req.Write(documentation)
}

func (h *DocumentationHandler) OpenAPI(req api.Context) error {
	route, err := ParseRoute(req.Request)
	if err != nil {
		writeHandlerError(req.ResponseWriter, req.Request.Method, err)
		return nil
	}
	var vmcp v1.VMCP
	if err := req.Get(&vmcp, route.VMCPID); err != nil {
		return fmt.Errorf("failed to get VMCP: %w", err)
	}
	instance, err := vmcpconfig.FindInstance(req.Context(), h.Storage, vmcp.Namespace, vmcp.Name, req.User.GetUID())
	if err != nil || instance == nil {
		return apiclienttypes.NewErrNotFound("VMCP connection not found")
	}

	components := vmcpconfig.ComponentsForInstance(vmcp, *instance)
	var selected *apiclienttypes.VMCPComponent
	for index := range components {
		component := &components[index]
		if component.ID == route.ComponentID && component.CatalogEntry.Manifest.Runtime == apiclienttypes.RuntimeOpenAPI {
			selected = component
			break
		}
	}
	if selected == nil {
		return apiclienttypes.NewErrNotFound("OpenAPI component not found")
	}

	description, err := h.Executor.Describe(req.Context(), req.User, Reference{VMCPID: vmcp.Name, ComponentID: selected.ID})
	if err != nil {
		return apiclienttypes.NewErrHTTP(httpStatusForDocumentation(err), "OpenAPI Worker documentation is unavailable")
	}
	basePath := strings.TrimSuffix(h.ServerURL, "/") + "/api-proxy/" + url.PathEscape(vmcp.Name) + "/" + url.PathEscape(selected.ID)
	documentation := ComponentDocumentation{
		ID:              selected.ID,
		DisplayName:     selected.Name,
		BasePath:        basePath,
		ExecutionDigest: description.ExecutionDigest,
		Operations:      operations(basePath, description),
	}
	return req.Write(newOpenAPIDocument(documentation))
}

type RouteHandler struct {
	Proxy         *Handler
	Documentation *DocumentationHandler
}

func (h *RouteHandler) Serve(req api.Context) error {
	route, err := ParseRoute(req.Request)
	if err == nil && req.Method == http.MethodGet && route.RawPath == "/openapi.json" {
		return h.Documentation.OpenAPI(req)
	}
	return h.Proxy.Proxy(req)
}

func httpStatusForDocumentation(err error) int {
	if proxyErr, ok := err.(*Error); ok && proxyErr.HTTPStatus >= 500 {
		return proxyErr.HTTPStatus
	}
	return 503
}

func operations(basePath string, description Description) []OperationDocumentation {
	businessAuthRequired := false
	for _, policy := range description.HeaderPolicies {
		if policy.Credential && policy.Required {
			businessAuthRequired = true
			break
		}
	}
	result := make([]OperationDocumentation, 0, len(description.Operations))
	for _, operation := range description.Operations {
		displayName := operation.DisplayName
		if displayName == "" {
			displayName = operation.LegacyToolName
		}
		result = append(result, OperationDocumentation{
			OperationKey:         operation.OperationKey,
			LegacyToolName:       operation.LegacyToolName,
			DisplayName:          displayName,
			Description:          operation.Description,
			Method:               operation.Method,
			PathTemplate:         operation.PathTemplate,
			Parameters:           operation.Parameters,
			RequestMediaTypes:    operation.RequestMediaTypes,
			BusinessAuthRequired: businessAuthRequired,
			Example:              operationExample(basePath, operation),
		})
	}
	return result
}

func newOpenAPIDocument(component ComponentDocumentation) OpenAPIDocument {
	paths := make(map[string]map[string]any, len(component.Operations))
	for _, operation := range component.Operations {
		method := strings.ToLower(operation.Method)
		if method == "" || operation.PathTemplate == "" {
			continue
		}
		if paths[operation.PathTemplate] == nil {
			paths[operation.PathTemplate] = make(map[string]any, 1)
		}

		openAPIOperation := map[string]any{
			"operationId": operation.LegacyToolName,
			"summary":     operation.DisplayName,
			"responses": map[string]any{
				"default": map[string]any{
					"description": "Business API response returned through Obot",
				},
			},
			"security": []map[string][]string{{"X-API-Key": {}}},
		}
		if operation.Description != "" {
			openAPIOperation["description"] = operation.Description
		}
		if len(operation.Parameters) > 0 {
			parameters := make([]map[string]any, 0, len(operation.Parameters))
			for _, parameter := range operation.Parameters {
				item := map[string]any{
					"name": parameter.Name,
					"in":   parameter.In,
				}
				if parameter.Required {
					item["required"] = true
				}
				if len(parameter.Schema) > 0 {
					item["schema"] = parameter.Schema
				}
				parameters = append(parameters, item)
			}
			openAPIOperation["parameters"] = parameters
		}
		if len(operation.RequestMediaTypes) > 0 {
			content := make(map[string]any, len(operation.RequestMediaTypes))
			for _, mediaType := range operation.RequestMediaTypes {
				content[mediaType] = map[string]any{}
			}
			openAPIOperation["requestBody"] = map[string]any{"content": content}
		}
		paths[operation.PathTemplate][method] = openAPIOperation
	}

	return OpenAPIDocument{
		OpenAPI: "3.1.0",
		Info: OpenAPIInfo{
			Title:       component.DisplayName,
			Version:     "1.0.0",
			Description: "Machine-readable contract for this API.",
		},
		Servers: []OpenAPIServer{{URL: component.BasePath}},
		Paths:   paths,
		Components: OpenAPIComponents{
			SecuritySchemes: map[string]OpenAPISecurityScheme{
				"X-API-Key": {Type: "apiKey", Name: "X-API-Key", In: "header"},
			},
		},
	}
}

func operationExample(basePath string, operation OperationDescription) string {
	path := operation.PathTemplate
	for _, parameter := range operation.Parameters {
		if parameter.In == "path" {
			path = strings.ReplaceAll(path, "{"+parameter.Name+"}", "<"+parameter.Name+">")
		}
	}
	return operation.Method + " " + basePath + path
}
