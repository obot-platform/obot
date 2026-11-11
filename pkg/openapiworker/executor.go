package openapiworker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/openapi/executionbudget"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
)

type Executor interface {
	Describe(ctx context.Context, principal PrincipalContext, reference Reference) (ResolvedDescription, error)
	Execute(ctx context.Context, principal PrincipalContext, invocation Invocation) (ExecutionResult, error)
}

type WorkerExecutor struct {
	resolver   *Resolver
	client     *Client
	budgets    BudgetManager
	instanceID string
}

type BudgetManager interface {
	Acquire(context.Context, executionbudget.AcquireRequest) (executionbudget.Acquisition, error)
	Release(context.Context, string, string, executionbudget.Outcome) error
}

func NewWorkerExecutor(resolver *Resolver, client *Client) *WorkerExecutor {
	return &WorkerExecutor{resolver: resolver, client: client, instanceID: defaultObotInstanceID()}
}

func (e *WorkerExecutor) ConfigureBudgetManager(manager BudgetManager) {
	e.budgets = manager
}

func (e *WorkerExecutor) Describe(ctx context.Context, principal PrincipalContext, reference Reference) (ResolvedDescription, error) {
	resolved, err := e.resolver.resolve(ctx, principal, reference)
	if err != nil {
		return ResolvedDescription{}, err
	}
	if _, err := e.client.EnsureSnapshotReady(ctx, resolved.snapshot); err != nil {
		return ResolvedDescription{}, err
	}
	return resolved.description, nil
}

func (e *WorkerExecutor) Execute(ctx context.Context, principal PrincipalContext, invocation Invocation) (ExecutionResult, error) {
	if invocation.InputKind == "" {
		invocation.InputKind = InputKindTool
	}
	if invocation.InputKind != InputKindTool && invocation.InputKind != InputKindHTTP {
		return ExecutionResult{}, NewError(ErrorCodeInvalidRequest, "unsupported execution input kind")
	}
	if invocation.Deadline.IsZero() {
		invocation.Deadline = time.Now().Add(defaultRequestTimeout)
	}
	if invocation.OperationKey == "" {
		return ExecutionResult{}, NewError(ErrorCodeInvalidRequest, "operation key is required")
	}
	if err := e.client.Ready(ctx); err != nil {
		return ExecutionResult{}, err
	}

	resolved, err := e.resolver.resolve(ctx, principal, invocation.Reference)
	if err != nil {
		return ExecutionResult{}, err
	}
	operation, err := e.authorizedOperation(resolved, invocation)
	if err != nil {
		return ExecutionResult{}, err
	}
	if operation.OperationKey != invocation.OperationKey {
		return ExecutionResult{}, NewError(ErrorCodeOperationNotFound, "operation does not match the resolved operation")
	}
	lease, releaseToken, err := e.acquireBudget(ctx, resolved.description, invocation.OperationKey, invocation.Deadline)
	if err != nil {
		return ExecutionResult{}, err
	}
	invocation.Budget.Lease = lease
	invocation.Deadline = time.UnixMilli(lease.FenceDeadlineUnixMs - 1)
	input, credentials, err := executionInput(resolved, invocation, operation)
	if err != nil {
		_ = e.budgets.Release(context.WithoutCancel(ctx), lease.LeaseID, releaseToken, executionbudget.OutcomePreUpstreamRejected)
		return ExecutionResult{}, err
	}
	request := ExecuteRequest{
		ProtocolVersion: ProtocolVersion,
		RequestID:       NewRequestID(),
		ExecutionDigest: resolved.snapshot.ExecutionDigest,
		OperationKey:    operation.OperationKey,
		Input:           input,
		DeadlineUnixMs:  invocation.Deadline.UnixMilli(),
		Lease:           invocation.Budget.Lease,
		Credentials:     credentials,
		Snapshot:        encodeSnapshot(resolved.snapshot),
	}
	ctx, cancel := context.WithDeadline(ctx, invocation.Deadline)
	defer cancel()
	result, executeErr := e.client.Execute(ctx, request)
	if executeErr == nil {
		_ = e.budgets.Release(context.WithoutCancel(ctx), lease.LeaseID, releaseToken, executionbudget.Outcome{
			Code:              executionbudget.OutcomeBusinessResult.Code,
			UpstreamAttempted: true,
			BusinessResult:    true,
			ResponseBytes:     int64(len(result.BodyBase64)),
		})
		return result, nil
	}
	if !upstreamAttempted(executeErr) {
		_ = e.budgets.Release(context.WithoutCancel(ctx), lease.LeaseID, releaseToken, executionbudget.OutcomePreUpstreamRejected)
	}
	return ExecutionResult{}, executeErr
}

func (e *WorkerExecutor) acquireBudget(ctx context.Context, description ResolvedDescription, operationKey string, deadline time.Time) (Lease, string, error) {
	if e.budgets == nil {
		return Lease{}, "", NewError(ErrorCodeBudgetIdentityUnavailable, "OpenAPI execution budget manager is unavailable")
	}
	identity, err := executionbudget.ResolveBudgetIdentity(
		[]executionbudget.CatalogReference{{SourceURL: description.BudgetSourceURL, EntryKey: description.BudgetEntryKey}},
		nil,
		operationKey,
	)
	if err != nil {
		return Lease{}, "", budgetError(err)
	}
	acquisition, err := e.budgets.Acquire(ctx, executionbudget.AcquireRequest{
		Identity:       identity,
		OperationKey:   operationKey,
		Deadline:       deadline,
		ObotInstanceID: e.instanceID,
	})
	if err != nil {
		return Lease{}, "", budgetError(err)
	}
	budgetContext := acquisition.Context
	return Lease{
		LeaseID:             budgetContext.LeaseID,
		LeaseIssuedAtUnixMs: budgetContext.LeaseIssuedAt.UnixMilli(),
		LeaseDeadlineUnixMs: budgetContext.LeaseDeadline.UnixMilli(),
		FenceDeadlineUnixMs: budgetContext.FenceDeadline.UnixMilli(),
		DispatchedAtUnixMs:  budgetContext.DispatchedAt.UnixMilli(),
		BudgetKeyHash:       budgetContext.BudgetKeyHash,
		ClockSkewLimitMs:    int64(budgetContext.ClockSkewLimit / time.Millisecond),
		FencingMarginMs:     int64(budgetContext.FencingMargin / time.Millisecond),
	}, acquisition.ReleaseToken, nil
}

func budgetError(err error) error {
	switch {
	case errors.Is(err, executionbudget.ErrQueueFull), errors.Is(err, executionbudget.ErrWaitDeadlineExceeded):
		return NewError(ErrorCodeBudgetRejected, "OpenAPI execution budget is exhausted")
	case errors.Is(err, executionbudget.ErrBudgetIdentityUnavailable):
		return NewError(ErrorCodeBudgetIdentityUnavailable, "OpenAPI budget identity is unavailable")
	default:
		return NewError(ErrorCodeWorkerUnavailable, "OpenAPI execution budget is unavailable")
	}
}

func upstreamAttempted(err error) bool {
	workerErr, ok := AsError(err)
	if !ok {
		return true
	}
	switch workerErr.Code {
	case ErrorCodePlatformUnauthenticated, ErrorCodePlatformForbidden, ErrorCodeResourceHidden,
		ErrorCodeOperationNotFound, ErrorCodeBusinessAuthorizationRequired, ErrorCodeInvalidRequest,
		ErrorCodeUnsupportedMediaType, ErrorCodeRequestTooLarge, ErrorCodeBudgetRejected,
		ErrorCodeBudgetIdentityUnavailable, ErrorCodeSnapshotRequired, ErrorCodeSnapshotInvalid,
		ErrorCodeAmbiguousOperation:
		return false
	default:
		return true
	}
}

func defaultObotInstanceID() string {
	if value := strings.TrimSpace(os.Getenv("OBOT_INSTANCE_ID")); value != "" {
		return value
	}
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		return hostname
	}
	return "obot-local"
}

func (e *WorkerExecutor) authorizedOperation(resolved resolvedExecution, invocation Invocation) (Operation, error) {
	var operation Operation
	if invocation.InputKind == InputKindHTTP {
		match, err := MatchOperation(resolved.manifest.Operations, invocation.HTTPInput.Method, invocation.HTTPInput.RawPath)
		if err != nil {
			return Operation{}, err
		}
		operation = match.Operation
	} else {
		for _, candidate := range resolved.manifest.Operations {
			if candidate.OperationKey == invocation.OperationKey {
				operation = candidate
				break
			}
		}
	}
	if operation.OperationKey == "" || !toolEnabled(operation.LegacyToolName, resolved.toolOverrides) {
		return Operation{}, NewError(ErrorCodeOperationNotFound, "operation is not authorized")
	}
	reference := types.VMCPToolReference{
		ComponentID: resolved.reference.ComponentID,
		Name:        operation.LegacyToolName,
	}
	if !vmcpaccess.ToolGranted(resolved.grant, reference) {
		return Operation{}, NewError(ErrorCodeOperationNotFound, "operation is not authorized")
	}
	return operation, nil
}

func executionInput(resolved resolvedExecution, invocation Invocation, operation Operation) (ExecuteInput, map[string]string, error) {
	input := ExecuteInput{Kind: invocation.InputKind}
	if invocation.InputKind == InputKindHTTP {
		headers := SanitizeInputHeaders(invocation.HTTPInput.Headers, resolved.description.HeaderPolicies)
		input.Method = strings.ToUpper(invocation.HTTPInput.Method)
		input.RawPath = invocation.HTTPInput.RawPath
		input.RawQuery = invocation.HTTPInput.RawQuery
		input.Headers = workerHeaders(headers)
		input.BodyBase64 = invocation.HTTPInput.BodyBase64
		input.BodyMediaType = invocation.HTTPInput.BodyMediaType
		return input, credentialsFromPolicies(resolved), nil
	}
	if len(invocation.ToolInput) > MaxRequestBodyBytes {
		return ExecuteInput{}, nil, NewError(ErrorCodeRequestTooLarge, "tool input exceeds 1 MiB")
	}
	input.Parameters = append(json.RawMessage(nil), invocation.ToolInput...)
	return input, credentialsFromPolicies(resolved), nil
}

func credentialsFromPolicies(resolved resolvedExecution) map[string]string {
	credentials := make(map[string]string, len(resolved.snapshot.Policy.Credentials))
	for _, policy := range resolved.snapshot.Policy.Credentials {
		values := resolved.credentialValues.Values(policy.Name)
		if len(values) > 0 {
			credentials[policy.Name] = values[len(values)-1]
		}
	}
	return credentials
}

func workerHeaders(headers http.Header) []HeaderValue {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]HeaderValue, 0, len(names))
	for _, name := range names {
		result = append(result, HeaderValue{Name: name, Values: headers.Values(name)})
	}
	return result
}

func encodeSnapshot(snapshot SnapshotEnvelope) json.RawMessage {
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return nil
	}
	return encoded
}

func (l Lease) validate(deadline time.Time) error {
	if l.LeaseID == "" || l.BudgetKeyHash == "" || l.LeaseIssuedAtUnixMs <= 0 ||
		l.LeaseDeadlineUnixMs <= 0 || l.FenceDeadlineUnixMs <= 0 || l.DispatchedAtUnixMs <= 0 ||
		l.ClockSkewLimitMs <= 0 || l.FencingMarginMs <= 0 {
		return NewError(ErrorCodeBudgetRejected, "worker lease fields are invalid")
	}
	if l.FencingMarginMs <= l.ClockSkewLimitMs ||
		l.FenceDeadlineUnixMs != l.LeaseDeadlineUnixMs-l.FencingMarginMs ||
		l.LeaseIssuedAtUnixMs > l.DispatchedAtUnixMs || l.DispatchedAtUnixMs >= l.FenceDeadlineUnixMs ||
		l.FenceDeadlineUnixMs < deadline.UnixMilli() {
		return NewError(ErrorCodeBudgetRejected, "worker lease is expired or invalid")
	}
	return nil
}

func (i HTTPInput) NormalizedMethod() string {
	return strings.ToUpper(i.Method)
}
