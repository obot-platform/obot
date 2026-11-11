package openapiworker_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/openapi/executionbudget"
	openapiworker "github.com/obot-platform/obot/pkg/openapiworker"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var schemaJSON = json.RawMessage(`{"openapi":"3.0.3","info":{"title":"Test API","version":"1"},"paths":{"/items/{id}":{"get":{"operationId":"getItem","parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}},{"name":"include","in":"query","required":false,"style":"form","explode":true,"schema":{"type":"boolean"}},{"name":"X-Business","in":"header","required":false,"schema":{"type":"string"}},{"name":"Authorization","in":"header","required":false,"schema":{"type":"string"}}]}}}}`)

func TestResolverExactIdentityAndCredentialContext(t *testing.T) {
	storage := newResolverStorage()
	credentials := newCredentialStore()
	resolver := openapiworker.NewResolver(storage, credentials, nil)

	userOne, principalOne, _ := openapiworker.NewPrincipalContext(t.Context(), userInfo("1", nil))
	description, err := resolver.Describe(userOne, principalOne, reference("connection-1"))
	require.NoError(t, err)
	operation := description.Operations[0]
	require.Equal(t, "GET /items/{id}", operation.OperationKey)
	require.Equal(t, http.MethodGet, operation.Method)
	require.Equal(t, "/items/{id}", operation.PathTemplate)
	require.NotEmpty(t, description.BudgetSourceID)
	require.Len(t, operation.Parameters, 4)
	require.Equal(t, "id", operation.Parameters[0].Name)
	require.Equal(t, "path", operation.Parameters[0].In)
	require.True(t, operation.Parameters[0].Required)
	require.JSONEq(t, `{"type":"string"}`, string(operation.Parameters[0].Schema))
	require.Equal(t, "include", operation.Parameters[1].Name)
	require.Equal(t, "query", operation.Parameters[1].In)
	require.Equal(t, "form", operation.Parameters[1].Style)
	require.True(t, operation.Parameters[1].Explode != nil && *operation.Parameters[1].Explode)

	match, err := openapiworker.MatchOperation([]openapiworker.Operation{{
		OperationKey: operation.OperationKey,
		Method:       operation.Method,
		PathTemplate: operation.PathTemplate,
	}}, http.MethodGet, "/items/a%2Fb")
	require.NoError(t, err)
	require.Equal(t, "a%2Fb", match.PathBindings["id"])
	require.Equal(t, operation.OperationKey, match.Operation.OperationKey)

	_, principalTwo, _ := openapiworker.NewPrincipalContext(t.Context(), userInfo("2", nil))
	_, err = resolver.Describe(t.Context(), principalTwo, reference("connection-1"))
	requireErrorCode(t, err, openapiworker.ErrorCodeResourceHidden)

	_, err = resolver.Describe(userOne, principalOne, reference("connection-1-missing"))
	requireErrorCode(t, err, openapiworker.ErrorCodeBusinessAuthorizationRequired)

	scoped := userInfo("1", []string{"other-vmcp"})
	scopedContext, scopedPrincipal, _ := openapiworker.NewPrincipalContext(t.Context(), scoped)
	_, err = resolver.Describe(scopedContext, scopedPrincipal, reference("connection-1"))
	requireErrorCode(t, err, openapiworker.ErrorCodePlatformForbidden)
}

func TestResolverDisabledToolAndSameUserSecondInstance(t *testing.T) {
	storage := newResolverStorage()
	storage.objects["vmcp"].(*v1.VMCP).Spec.Manifest.Components[0].ToolOverrides = []types.ToolOverride{{Name: "getItem", Enabled: false}}
	resolver := openapiworker.NewResolver(storage, newCredentialStore(), nil)

	ctx, principal, _ := openapiworker.NewPrincipalContext(t.Context(), userInfo("1", nil))
	description, err := resolver.Describe(ctx, principal, reference("connection-1"))
	require.NoError(t, err)
	require.Empty(t, description.Operations)
}

func TestExecutorSendsExactCredentialOnlyToWorker(t *testing.T) {
	var executeRequest openapiworker.ExecuteRequest
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/readyz":
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		case "/internal/openapi/v1/describe":
			var request openapiworker.DescribeRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			_ = json.NewEncoder(w).Encode(openapiworker.DescribeResponse{
				Kind: "describe", Status: "ready", ProtocolVersion: openapiworker.ProtocolVersion,
				ExecutionDigest: request.ExecutionDigest, TotalOperations: 1, Complete: true,
			})
		case "/internal/openapi/v1/execute":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&executeRequest))
			_ = json.NewEncoder(w).Encode(openapiworker.ExecutionResult{
				Kind: openapiworker.ResultKindBusiness, RequestID: executeRequest.RequestID,
				Status: http.StatusTeapot, Outcome: "business_result",
				Headers:    []openapiworker.HeaderValue{{Name: "X-Business", Values: []string{"response"}}},
				BodyBase64: "e30=",
			})
		default:
			t.Errorf("unexpected worker path %s", r.URL.Path)
		}
	}))
	defer worker.Close()

	client, err := openapiworker.NewClient(openapiworker.ClientConfig{BaseURL: worker.URL, DevMode: true})
	require.NoError(t, err)
	defer client.Close()
	resolver := openapiworker.NewResolver(newResolverStorage(), newCredentialStore(), nil)
	executor := openapiworker.NewWorkerExecutor(resolver, client)
	executor.ConfigureBudgetManager(testBudgetManager{})
	ctx, principal, _ := openapiworker.NewPrincipalContext(t.Context(), userInfo("1", nil))

	lease := testLease(25 * time.Second)
	result, err := executor.Execute(ctx, principal, openapiworker.Invocation{
		Reference:    reference("connection-1"),
		OperationKey: "GET /items/{id}",
		InputKind:    openapiworker.InputKindTool,
		ToolInput:    json.RawMessage(`{"id":"42"}`),
		Deadline:     time.UnixMilli(lease.FenceDeadlineUnixMs - 1),
		Budget:       openapiworker.BudgetContext{Lease: lease},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusTeapot, result.Status)
	require.Equal(t, "Bearer user-one", executeRequest.Credentials["Authorization"])
	require.Equal(t, "test-lease", executeRequest.Lease.LeaseID)
	require.Equal(t, "test-budget-key", executeRequest.Lease.BudgetKeyHash)
	require.Equal(t, openapiworker.InputKindTool, executeRequest.Input.Kind)
	require.JSONEq(t, `{"id":"42"}`, string(executeRequest.Input.Parameters))
	require.NotEmpty(t, executeRequest.Snapshot)
}

func TestExecutorRejectsInvalidOperationWithoutWorkerExecute(t *testing.T) {
	var ready, executes atomic.Int64
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/readyz":
			ready.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		case "/internal/openapi/v1/execute":
			executes.Add(1)
			t.Errorf("unexpected worker execute")
		default:
			http.NotFound(w, r)
		}
	}))
	defer worker.Close()

	client, err := openapiworker.NewClient(openapiworker.ClientConfig{BaseURL: worker.URL, DevMode: true})
	require.NoError(t, err)
	defer client.Close()
	resolver := openapiworker.NewResolver(newResolverStorage(), newCredentialStore(), nil)
	executor := openapiworker.NewWorkerExecutor(resolver, client)
	executor.ConfigureBudgetManager(testBudgetManager{})
	ctx, principal, _ := openapiworker.NewPrincipalContext(t.Context(), userInfo("1", nil))
	lease := testLease(25 * time.Second)

	_, err = executor.Execute(ctx, principal, openapiworker.Invocation{
		Reference: reference("connection-1"),
		InputKind: openapiworker.InputKindTool,
		Deadline:  time.UnixMilli(lease.FenceDeadlineUnixMs - 1),
		Budget:    openapiworker.BudgetContext{Lease: lease},
	})
	requireErrorCode(t, err, openapiworker.ErrorCodeInvalidRequest)
	require.Zero(t, ready.Load())

	_, err = executor.Execute(ctx, principal, openapiworker.Invocation{
		Reference:    reference("connection-1"),
		OperationKey: "GET /other/{id}",
		InputKind:    openapiworker.InputKindHTTP,
		HTTPInput: openapiworker.HTTPInput{
			Method: http.MethodGet, RawPath: "/items/42", Headers: http.Header{},
		},
		Deadline: time.UnixMilli(lease.FenceDeadlineUnixMs - 1),
		Budget:   openapiworker.BudgetContext{Lease: lease},
	})
	requireErrorCode(t, err, openapiworker.ErrorCodeOperationNotFound)
	require.EqualValues(t, 1, ready.Load())
	require.Zero(t, executes.Load())
}

func testLease(duration time.Duration) openapiworker.Lease {
	now := time.Now().UnixMilli()
	remaining := duration.Milliseconds()
	if remaining < 20_000 {
		remaining = 20_000
	}
	return openapiworker.Lease{
		LeaseID: "lease-1", LeaseIssuedAtUnixMs: now - 10,
		LeaseDeadlineUnixMs: now + remaining,
		FenceDeadlineUnixMs: now + remaining - 10000,
		DispatchedAtUnixMs:  now, BudgetKeyHash: strings.Repeat("a", 64),
		ClockSkewLimitMs: 5000, FencingMarginMs: 10000,
	}
}

func reference(connectionID string) openapiworker.Reference {
	return openapiworker.Reference{VMCPID: "vmcp", ComponentID: "component", InstanceName: connectionID}
}

func userInfo(userID string, authorizedMCPIDs []string) kuser.Info {
	info := &kuser.DefaultInfo{
		Name: userID, UID: userID,
		Groups: []string{types.GroupAuthenticated},
		Extra: map[string][]string{
			"obot_groups":          {},
			"auth_provider_groups": {},
		},
	}
	if authorizedMCPIDs != nil {
		info.Extra["authorized_mcp_ids"] = authorizedMCPIDs
	}
	return info
}

type fakeStorage struct{ objects map[string]kclient.Object }

func newResolverStorage() *fakeStorage {
	vmcp := &v1.VMCP{
		ObjectMeta: metav1Object("vmcp"),
		Spec: v1.VMCPSpec{
			Manifest: types.VMCPManifest{
				Components: []types.VMCPComponent{component(false)},
				Profiles: []types.VMCPProfile{{
					Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
					Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
				}},
			},
			ComponentCatalogReferences: map[string]string{"component": "https://source.example.test/catalog.json::api"},
		},
	}
	storage := &fakeStorage{objects: map[string]kclient.Object{
		"vmcp":                            vmcp,
		"server":                          openAPIServer("vmcp", ""),
		"vmcp-instance-1":                 vmcpInstance("1"),
		"vmcp-instance-2":                 vmcpInstance("1"),
		"vmcp-instance-user-2":            vmcpInstance("2"),
		"vmcp-instance-unauthorized-tool": vmcpInstance("1"),
		"connection-1":                    connection("1", "vmcp-instance-1"),
		"connection-2":                    connection("2", "vmcp-instance-user-2"),
		"connection-1-missing":            connection("1", "vmcp-instance-2"),
		"connection-1-unauthorized-tool":  connection("1", "vmcp-instance-unauthorized-tool"),
	}}
	storage.objects["vmcp-instance-2"].SetName("vmcp-instance-2")
	storage.objects["vmcp-instance-unauthorized-tool"].SetName("vmcp-instance-unauthorized-tool")
	storage.objects["connection-1-missing"].SetName("connection-1-missing")
	storage.objects["connection-1-unauthorized-tool"].SetName("connection-1-unauthorized-tool")
	return storage
}

func component(disabled bool) types.VMCPComponent {
	overrides := []types.ToolOverride{{Name: "getItem", Enabled: true}}
	if disabled {
		overrides[0].Enabled = false
	}
	return types.VMCPComponent{
		ID: "component", Name: "API",
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
			Name: "API", Runtime: types.RuntimeOpenAPI,
			OpenAPIConfig: &types.OpenAPIRuntimeConfig{
				BaseURL: "https://api.example.test", Schema: &types.OpenAPISchema{Raw: schemaJSON},
			},
			Config: []types.MCPConfig{{
				Name: "Authorization", Key: "Authorization", Required: true, Sensitive: true,
				Prefix: "Bearer ", Usage: types.Header, UserAllowed: true,
			}},
		}},
		ToolOverrides: overrides,
	}
}

type testBudgetManager struct{}

func (testBudgetManager) Acquire(_ context.Context, request executionbudget.AcquireRequest) (executionbudget.Acquisition, error) {
	now := time.Now().UTC()
	return executionbudget.Acquisition{
		Context: executionbudget.BudgetContext{
			LeaseID:        "test-lease",
			LeaseIssuedAt:  now.Add(-time.Second),
			LeaseDeadline:  request.Deadline.UTC(),
			FenceDeadline:  request.Deadline.Add(-2 * time.Second).UTC(),
			DispatchedAt:   now,
			BudgetKeyHash:  "test-budget-key",
			ClockSkewLimit: time.Second,
			FencingMargin:  2 * time.Second,
		},
		ReleaseToken: "test-release-token",
	}, nil
}

func (testBudgetManager) Release(context.Context, string, string, executionbudget.Outcome) error {
	return nil
}

func openAPIServer(vmcpID, vmcpInstanceID string) *v1.MCPServer {
	return &v1.MCPServer{
		ObjectMeta: metav1Object("server"),
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{Name: "API", Runtime: types.RuntimeOpenAPI},
			VMCPID:   vmcpID, VMCPComponentID: "component", VMCPInstanceID: vmcpInstanceID,
		},
	}
}

func vmcpInstance(userID string) *v1.VMCPInstance {
	return &v1.VMCPInstance{
		ObjectMeta: metav1Object(fmt.Sprintf("vmcp-instance-%s", userID)),
		Spec:       v1.VMCPInstanceSpec{Manifest: types.VMCPInstanceManifest{VMCPID: "vmcp"}, UserID: userID},
	}
}

func connection(userID, vmcpInstanceID string) *v1.MCPServerInstance {
	return &v1.MCPServerInstance{
		ObjectMeta: metav1Object(fmt.Sprintf("connection-%s", userID)),
		Spec: v1.MCPServerInstanceSpec{
			VMCPInstanceID: vmcpInstanceID, VMCPComponentID: "component", UserID: userID,
			MCPServerName: "server",
			Config: []types.MCPConfig{{
				Name: "Authorization", Key: "Authorization", Required: true, Sensitive: true,
				Prefix: "Bearer ", Usage: types.Header, UserAllowed: true,
			}},
		},
	}
}

func metav1Object(name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name}
}

func (s *fakeStorage) Get(_ context.Context, key kclient.ObjectKey, object kclient.Object, _ ...kclient.GetOption) error {
	if key.Namespace != system.DefaultNamespace {
		return apierrors.NewNotFound(schema.GroupResource{}, key.Name)
	}
	stored, ok := s.objects[key.Name]
	if !ok {
		return apierrors.NewNotFound(schema.GroupResource{}, key.Name)
	}
	switch target := object.(type) {
	case *v1.VMCP:
		*target = *stored.(*v1.VMCP)
	case *v1.VMCPInstance:
		*target = *stored.(*v1.VMCPInstance)
	case *v1.MCPServerInstance:
		*target = *stored.(*v1.MCPServerInstance)
	case *v1.MCPServer:
		*target = *stored.(*v1.MCPServer)
	default:
		return fmt.Errorf("unsupported object %T", object)
	}
	return nil
}

type fakeCredentials struct{}

func newCredentialStore() fakeCredentials { return fakeCredentials{} }

func (fakeCredentials) RevealCredential(_ context.Context, contexts []string, _ string) (gatewaytypes.Credential, error) {
	if len(contexts) == 0 {
		return gatewaytypes.Credential{}, gorm.ErrRecordNotFound
	}
	switch contexts[0] {
	case "1-connection-1", "1-connection-1-missing", "1-connection-1-unauthorized-tool", "2-connection-2":
		if contexts[0] == "1-connection-1-missing" {
			return gatewaytypes.Credential{}, gorm.ErrRecordNotFound
		}
		if contexts[0] == "2-connection-2" {
			return gatewaytypes.Credential{Secrets: map[string]string{"Authorization": "user-two"}}, nil
		}
		return gatewaytypes.Credential{Secrets: map[string]string{"Authorization": "user-one"}}, nil
	default:
		return gatewaytypes.Credential{}, gorm.ErrRecordNotFound
	}
}
