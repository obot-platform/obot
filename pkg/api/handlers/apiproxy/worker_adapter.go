package apiproxy

import (
	"context"
	"encoding/base64"

	"github.com/obot-platform/obot/pkg/openapiworker"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type WorkerExecutorAdapter struct {
	executor openapiworker.Executor
	storage  kclient.Client
}

func NewWorkerExecutorAdapter(executor openapiworker.Executor, storage kclient.Client) *WorkerExecutorAdapter {
	return &WorkerExecutorAdapter{executor: executor, storage: storage}
}

func (a *WorkerExecutorAdapter) Describe(ctx context.Context, principal kuser.Info, reference Reference) (Description, error) {
	workerReference, err := a.workerReference(ctx, principal, reference)
	if err != nil {
		return Description{}, err
	}
	principalContext, workerPrincipal, err := openapiworker.NewPrincipalContext(ctx, principal)
	if err != nil {
		return Description{}, proxyError(err)
	}
	description, err := a.executor.Describe(principalContext, workerPrincipal, workerReference)
	if err != nil {
		return Description{}, proxyError(err)
	}
	return mapDescription(description), nil
}

func (a *WorkerExecutorAdapter) Execute(ctx context.Context, principal kuser.Info, invocation Invocation) (ExecutionResult, error) {
	workerReference, err := a.workerReference(ctx, principal, invocation.Reference)
	if err != nil {
		return ExecutionResult{}, err
	}
	principalContext, workerPrincipal, err := openapiworker.NewPrincipalContext(ctx, principal)
	if err != nil {
		return ExecutionResult{}, proxyError(err)
	}
	result, executeErr := a.executor.Execute(principalContext, workerPrincipal, openapiworker.Invocation{
		Reference:    workerReference,
		OperationKey: invocation.OperationKey,
		InputKind:    openapiworker.InputKindHTTP,
		HTTPInput: openapiworker.HTTPInput{
			Method:        invocation.Method,
			RawPath:       invocation.RawPath,
			RawQuery:      invocation.RawQuery,
			Headers:       invocation.Headers,
			BodyBase64:    base64.StdEncoding.EncodeToString(invocation.Body),
			BodyMediaType: invocation.BodyMimeType,
		},
		Deadline: invocation.Deadline,
	})
	if executeErr != nil {
		return ExecutionResult{}, proxyError(executeErr)
	}
	body, decodeErr := openapiworker.DecodeBase64(result.BodyBase64)
	if decodeErr != nil {
		return ExecutionResult{}, proxyError(decodeErr)
	}
	return ExecutionResult{
		Kind:      string(result.Kind),
		RequestID: result.RequestID,
		Status:    result.Status,
		Headers:   mapHeaders(result.Headers),
		Body:      body,
		Outcome:   result.Outcome,
	}, nil
}

func (a *WorkerExecutorAdapter) workerReference(ctx context.Context, principal kuser.Info, reference Reference) (openapiworker.Reference, error) {
	if principal == nil || principal.GetUID() == "" {
		return openapiworker.Reference{}, NewError("platform_unauthenticated", "authenticated user is required")
	}
	instance, err := vmcpconfig.FindInstance(ctx, a.storage, system.DefaultNamespace, reference.VMCPID, principal.GetUID())
	if err != nil || instance == nil {
		return openapiworker.Reference{}, NewError("resource_hidden", "OpenAPI vMCP instance is unavailable")
	}
	var vmcp v1.VMCP
	if err := a.storage.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: reference.VMCPID}, &vmcp); err != nil {
		return openapiworker.Reference{}, NewError("resource_hidden", "OpenAPI vMCP is unavailable")
	}
	var connections v1.MCPServerInstanceList
	if err := a.storage.List(ctx, &connections, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return openapiworker.Reference{}, NewError("resource_hidden", "OpenAPI component connections are unavailable")
	}
	for _, component := range vmcpconfig.ComponentsForInstance(vmcp, *instance) {
		if component.ID == reference.ComponentID {
			connectionID := ""
			for _, connection := range connections.Items {
				if connection.DeletionTimestamp.IsZero() &&
					connection.Spec.UserID == principal.GetUID() &&
					connection.Spec.VMCPInstanceID == instance.Name &&
					connection.Spec.VMCPComponentID == reference.ComponentID {
					connectionID = connection.Name
					break
				}
			}
			if connectionID == "" {
				return openapiworker.Reference{}, NewError("resource_hidden", "OpenAPI component connection is unavailable")
			}
			return openapiworker.Reference{
				VMCPID:       reference.VMCPID,
				ComponentID:  reference.ComponentID,
				InstanceName: connectionID,
			}, nil
		}
	}
	return openapiworker.Reference{}, NewError("resource_hidden", "OpenAPI component is not selected for this instance")
}

func mapDescription(description openapiworker.ResolvedDescription) Description {
	operations := make([]OperationDescription, 0, len(description.Operations))
	for _, operation := range description.Operations {
		operations = append(operations, OperationDescription{
			Operation: Operation{
				OperationKey:   operation.OperationKey,
				LegacyToolName: operation.LegacyToolName,
				Method:         operation.Method,
				PathTemplate:   operation.PathTemplate,
			},
			DisplayName:       operation.DisplayName,
			Description:       operation.Description,
			Parameters:        mapParameters(operation.Parameters),
			RequestMediaTypes: operation.RequestMediaTypes,
		})
	}
	policies := make([]HeaderPolicy, 0, len(description.HeaderPolicies))
	for _, policy := range description.HeaderPolicies {
		policies = append(policies, HeaderPolicy{
			Name:        policy.Name,
			Required:    policy.Required,
			Forwardable: policy.Forwardable,
			Sensitive:   policy.Sensitive,
			Credential:  policy.Credential,
		})
	}
	return Description{ExecutionDigest: description.ExecutionDigest, Operations: operations, HeaderPolicies: policies}
}

func mapParameters(parameters []openapiworker.Parameter) []ParameterDocumentation {
	if len(parameters) == 0 {
		return nil
	}
	result := make([]ParameterDocumentation, 0, len(parameters))
	for _, parameter := range parameters {
		result = append(result, ParameterDocumentation{
			Name:     parameter.Name,
			In:       parameter.In,
			Required: parameter.Required,
			Schema:   parameter.Schema,
		})
	}
	return result
}

func mapHeaders(headers []openapiworker.HeaderValue) []HeaderValue {
	result := make([]HeaderValue, 0, len(headers))
	for _, header := range headers {
		result = append(result, HeaderValue{Name: header.Name, Values: header.Values})
	}
	return result
}

func proxyError(err error) error {
	if workerErr, ok := openapiworker.AsError(err); ok {
		return &Error{
			Code:       string(workerErr.Code),
			Message:    workerErr.Message,
			HTTPStatus: workerErr.HTTPStatus,
			RequestID:  workerErr.RequestID,
			Retryable:  workerErr.Retryable,
		}
	}
	return NewError("platform_error", "OpenAPI worker request failed")
}
