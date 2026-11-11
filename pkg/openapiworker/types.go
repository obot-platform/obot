package openapiworker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	kuser "k8s.io/apiserver/pkg/authentication/user"
)

const (
	ProtocolVersion       = 1
	digestDomain          = "obot-openapi-worker-v1"
	MaxSchemaBytes        = 1 << 20
	MaxRequestBodyBytes   = 1 << 20
	MaxResponseBodyBytes  = 8 << 20
	workerAuthHeaderName  = "X-Obot-Worker-Authentication"
	describePath          = "/internal/openapi/v1/describe"
	executePath           = "/internal/openapi/v1/execute"
	defaultRequestTimeout = 30 * time.Second
)

type Reference struct {
	VMCPID       string `json:"vmcpID"`
	ComponentID  string `json:"componentID"`
	InstanceName string `json:"instanceName"`
}

type principalContextKey struct{}

type PrincipalContext struct {
	user            kuser.Info
	authorizedVMCPs []string
	apiKey          bool
}

func NewPrincipalContext(ctx context.Context, userInfo kuser.Info) (context.Context, PrincipalContext, error) {
	if userInfo == nil || strings.TrimSpace(userInfo.GetUID()) == "" {
		return ctx, PrincipalContext{}, NewError(ErrorCodePlatformUnauthenticated, "authenticated user is required")
	}
	principal := PrincipalContext{user: userInfo}
	if scopes := userInfo.GetExtra()["authorized_mcp_ids"]; len(scopes) > 0 {
		principal.apiKey = true
		principal.authorizedVMCPs = append(principal.authorizedVMCPs, scopes...)
	}
	return context.WithValue(ctx, principalContextKey{}, principal), principal, nil
}

func PrincipalFromContext(ctx context.Context) (PrincipalContext, error) {
	principal, ok := ctx.Value(principalContextKey{}).(PrincipalContext)
	if !ok || principal.user == nil || principal.user.GetUID() == "" {
		return PrincipalContext{}, NewError(ErrorCodePlatformUnauthenticated, "authenticated user is required")
	}
	return principal, nil
}

func (p PrincipalContext) UserID() string {
	if p.user == nil {
		return ""
	}
	return p.user.GetUID()
}

func (p PrincipalContext) UserInfo() kuser.Info {
	return p.user
}

func (p PrincipalContext) withUserInfo(userInfo kuser.Info) PrincipalContext {
	if userInfo == nil || userInfo.GetUID() == "" {
		return p
	}
	return PrincipalContext{user: userInfo, authorizedVMCPs: p.authorizedVMCPs, apiKey: p.apiKey}
}

type InputKind string

const (
	InputKindTool InputKind = "tool"
	InputKindHTTP InputKind = "http"
)

type Invocation struct {
	Reference    Reference
	OperationKey string
	InputKind    InputKind
	ToolInput    json.RawMessage
	HTTPInput    HTTPInput
	Deadline     time.Time
	Budget       BudgetContext
}

type BudgetContext struct {
	Lease Lease `json:"lease"`
}

type Lease struct {
	LeaseID             string `json:"leaseID"`
	LeaseIssuedAtUnixMs int64  `json:"leaseIssuedAtUnixMs"`
	LeaseDeadlineUnixMs int64  `json:"leaseDeadlineUnixMs"`
	FenceDeadlineUnixMs int64  `json:"fenceDeadlineUnixMs"`
	DispatchedAtUnixMs  int64  `json:"dispatchedAtUnixMs"`
	BudgetKeyHash       string `json:"budgetKeyHash"`
	ClockSkewLimitMs    int64  `json:"clockSkewLimitMs"`
	FencingMarginMs     int64  `json:"fencingMarginMs"`
}

type HTTPInput struct {
	Method        string        `json:"method"`
	RawPath       string        `json:"rawPath"`
	RawQuery      string        `json:"rawQuery,omitempty"`
	Headers       http.Header   `json:"-"`
	WorkerHeaders []HeaderValue `json:"headers,omitempty"`
	BodyBase64    string        `json:"bodyBase64,omitempty"`
	BodyMediaType string        `json:"bodyMediaType,omitempty"`
}

type ResultKind string

const (
	ResultKindBusiness ResultKind = "business"
	ResultKindTool     ResultKind = "tool"
)

type ExecutionResult struct {
	Kind       ResultKind      `json:"kind"`
	RequestID  string          `json:"requestID"`
	Status     int             `json:"status,omitempty"`
	Headers    []HeaderValue   `json:"headers,omitempty"`
	BodyBase64 string          `json:"bodyBase64,omitempty"`
	ToolOutput json.RawMessage `json:"toolOutput,omitempty"`
	Outcome    string          `json:"outcome,omitempty"`
}

type HeaderValue struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

type DescribeRequest struct {
	ProtocolVersion int             `json:"protocolVersion"`
	RequestID       string          `json:"requestID"`
	ExecutionDigest string          `json:"executionDigest"`
	Snapshot        json.RawMessage `json:"snapshot,omitempty"`
	Limit           int             `json:"limit,omitempty"`
	Cursor          int             `json:"cursor,omitempty"`
}

type OperationDescription struct {
	OperationKey   string          `json:"operationKey"`
	LegacyToolName string          `json:"legacyToolName"`
	Method         string          `json:"method"`
	PathTemplate   string          `json:"pathTemplate"`
	DisplayName    string          `json:"displayName"`
	Description    string          `json:"description,omitempty"`
	InputSchema    json.RawMessage `json:"inputSchema,omitempty"`
	Parameters     []Parameter     `json:"parameters,omitempty"`
}

type DescribeResponse struct {
	Kind            string            `json:"kind"`
	Status          string            `json:"status"`
	ProtocolVersion int               `json:"protocolVersion"`
	ExecutionDigest string            `json:"executionDigest"`
	Operations      []WorkerOperation `json:"operations"`
	NextCursor      *int              `json:"nextCursor"`
	TotalOperations int               `json:"totalOperations"`
	Complete        bool              `json:"complete"`
}

type WorkerOperation struct {
	OperationKey   string          `json:"operationKey"`
	LegacyToolName string          `json:"legacyToolName"`
	Method         string          `json:"method"`
	PathTemplate   string          `json:"pathTemplate"`
	InputSchema    json.RawMessage `json:"inputSchema"`
	Parameters     []Parameter     `json:"parameters"`
}

type ExecuteRequest struct {
	ProtocolVersion int               `json:"protocolVersion"`
	RequestID       string            `json:"requestID"`
	ExecutionDigest string            `json:"executionDigest"`
	OperationKey    string            `json:"operationKey"`
	Input           ExecuteInput      `json:"input"`
	DeadlineUnixMs  int64             `json:"deadlineUnixMs"`
	Lease           Lease             `json:"lease"`
	Credentials     map[string]string `json:"credentials,omitempty"`
	Snapshot        json.RawMessage   `json:"snapshot,omitempty"`
}

type ExecuteInput struct {
	Kind          InputKind       `json:"kind"`
	Parameters    json.RawMessage `json:"parameters,omitempty"`
	Method        string          `json:"method,omitempty"`
	RawPath       string          `json:"rawPath,omitempty"`
	RawQuery      string          `json:"rawQuery,omitempty"`
	Headers       []HeaderValue   `json:"headers,omitempty"`
	BodyBase64    string          `json:"bodyBase64,omitempty"`
	BodyMediaType string          `json:"bodyMediaType,omitempty"`
}

type SnapshotEnvelope struct {
	ProtocolVersion int    `json:"protocolVersion"`
	ExecutionDigest string `json:"executionDigest"`
	ManifestBase64  string `json:"manifestBase64"`
	PolicyBase64    string `json:"policyBase64"`
	SchemaBase64    string `json:"schemaBase64"`

	SchemaDigest string               `json:"-"`
	Manifest     json.RawMessage      `json:"-"`
	Policy       HeaderPolicyDocument `json:"-"`
}

func (e SnapshotEnvelope) Clone() SnapshotEnvelope {
	var forwardable []HeaderPolicyDefinition
	if e.Policy.Forwardable != nil {
		forwardable = append([]HeaderPolicyDefinition{}, e.Policy.Forwardable...)
	}
	var credentials []HeaderPolicyDefinition
	if e.Policy.Credentials != nil {
		credentials = append([]HeaderPolicyDefinition{}, e.Policy.Credentials...)
	}
	return SnapshotEnvelope{
		ProtocolVersion: e.ProtocolVersion,
		SchemaBase64:    e.SchemaBase64,
		SchemaDigest:    e.SchemaDigest,
		Manifest:        append(json.RawMessage(nil), e.Manifest...),
		ManifestBase64:  e.ManifestBase64,
		PolicyBase64:    e.PolicyBase64,
		Policy:          HeaderPolicyDocument{Forwardable: forwardable, Credentials: credentials},
		ExecutionDigest: e.ExecutionDigest,
	}
}

func CredentialContext(userID, instanceName string) string {
	return fmt.Sprintf("%s-%s", userID, instanceName)
}
