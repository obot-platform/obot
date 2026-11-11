package openapiworker_test

import (
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	openapiworker "github.com/obot-platform/obot/pkg/openapiworker"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
)

func TestRealI01PythonWorkerDescribeAndExecute(t *testing.T) {
	workerRoot := os.Getenv("I01_OPENAPI_WORKER_ROOT")
	if workerRoot == "" {
		t.Skip("set I01_OPENAPI_WORKER_ROOT to run the real Python Worker integration test")
	}

	observedHeaders := make(chan http.Header, 1)
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/items/42" {
			http.NotFound(w, r)
			return
		}
		select {
		case observedHeaders <- r.Header.Clone():
		default:
		}
		if r.Header.Get("Authorization") == "Bearer user-two" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Business", "denied")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"ok":false}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Business", "accepted")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()
	certificatePath := filepath.Join(t.TempDir(), "upstream-ca.pem")
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: upstream.Certificate().Raw})
	require.NoError(t, os.WriteFile(certificatePath, certificatePEM, 0o600))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	workerPort := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	pythonBinary := filepath.Join(workerRoot, ".venv", "bin", "python")
	command := exec.Command(pythonBinary, "-m", "uvicorn", "openapi_worker.app:app", "--host", "127.0.0.1", "--port", fmt.Sprint(workerPort), "--log-level", "warning")
	command.Dir = workerRoot
	command.Env = append(os.Environ(),
		"OPENAPI_WORKER_TEST_HTTP=1",
		"OPENAPI_WORKER_ALLOW_LOCAL_TEST_EGRESS=1",
		"SSL_CERT_FILE="+certificatePath,
	)
	command.Stderr = os.Stderr
	require.NoError(t, command.Start())
	t.Cleanup(func() {
		_ = command.Process.Signal(syscall.SIGTERM)
		_ = command.Wait()
	})

	workerClient, err := openapiworker.NewClient(openapiworker.ClientConfig{
		BaseURL: fmt.Sprintf("http://127.0.0.1:%d", workerPort), DevMode: true,
	})
	require.NoError(t, err)
	defer workerClient.Close()
	require.Eventually(t, func() bool {
		return workerClient.Ready(t.Context()) == nil
	}, 10*time.Second, 100*time.Millisecond, "real I01 Python Worker did not become ready")

	storage := newResolverStorage()
	vmcp := storage.objects["vmcp"].(*v1.VMCP)
	vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest.OpenAPIConfig.BaseURL = upstream.URL
	resolver := openapiworker.NewResolver(storage, newCredentialStore(), nil)
	executor := openapiworker.NewWorkerExecutor(resolver, workerClient)
	executor.ConfigureBudgetManager(testBudgetManager{})
	ctx, principal, err := openapiworker.NewPrincipalContext(t.Context(), userInfo("1", nil))
	require.NoError(t, err)

	description, err := executor.Describe(ctx, principal, reference("connection-1"))
	require.NoError(t, err)
	descriptionOperation := description.Operations[0]
	require.Equal(t, "GET /items/{id}", descriptionOperation.OperationKey)
	require.Equal(t, http.MethodGet, descriptionOperation.Method)
	require.Equal(t, "/items/{id}", descriptionOperation.PathTemplate)
	require.NotEmpty(t, descriptionOperation.Parameters)
	lease := testLease(25 * time.Second)
	deadline := time.UnixMilli(lease.FenceDeadlineUnixMs - 1)

	result, err := executor.Execute(ctx, principal, openapiworker.Invocation{
		Reference:    reference("connection-1"),
		OperationKey: "GET /items/{id}",
		InputKind:    openapiworker.InputKindTool,
		ToolInput:    []byte(`{"id":"42","X-Business":"forwarded","Authorization":"caller"}`),
		Deadline:     deadline,
		Budget:       openapiworker.BudgetContext{Lease: lease},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, result.Status)
	require.Len(t, observedHeaders, 1)
	observed := <-observedHeaders
	require.Equal(t, "Bearer user-one", observed.Get("Authorization"))
	require.Equal(t, "forwarded", observed.Get("X-Business"))
	require.Empty(t, observed.Get("X-API-Key"))
	require.Empty(t, observed.Get("X-Obot-Trace"))
	require.Equal(t, "eyJvayI6dHJ1ZX0=", result.BodyBase64)

	ctxTwo, principalTwo, err := openapiworker.NewPrincipalContext(t.Context(), userInfo("2", nil))
	require.NoError(t, err)
	resultTwo, err := executor.Execute(ctxTwo, principalTwo, openapiworker.Invocation{
		Reference:    reference("connection-2"),
		OperationKey: "GET /items/{id}",
		InputKind:    openapiworker.InputKindTool,
		ToolInput:    []byte(`{"id":"42","Authorization":"caller"}`),
		Deadline:     time.Now().Add(25 * time.Second),
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, resultTwo.Status)
	require.Equal(t, "eyJvayI6ZmFsc2V9", resultTwo.BodyBase64)
	require.Len(t, observedHeaders, 1)
	observedTwo := <-observedHeaders
	require.Equal(t, "Bearer user-two", observedTwo.Get("Authorization"))
}
