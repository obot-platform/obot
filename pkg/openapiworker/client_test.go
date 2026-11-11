package openapiworker_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/openapiworker"
	"github.com/stretchr/testify/require"
)

type testCertificateAuthority struct {
	certificate *x509.Certificate
	privateKey  *rsa.PrivateKey
}

func writeCA(t *testing.T) (testCertificateAuthority, string) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "OpenAPI Worker test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)
	certificate, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	caFile := writePEM(t, "CERTIFICATE", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	return testCertificateAuthority{certificate: certificate, privateKey: privateKey}, caFile
}

func writeLeafCertificate(t *testing.T, authority testCertificateAuthority, subject pkix.Name, usage x509.ExtKeyUsage, dnsNames []string, ipAddresses []net.IP, notAfter func() time.Time) testCertificate {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      subject,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter(),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
		DNSNames:     dnsNames,
		IPAddresses:  ipAddresses,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, authority.certificate, &privateKey.PublicKey, authority.privateKey)
	require.NoError(t, err)
	certificate, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return testCertificate{
		certificate:     certificate,
		certificateFile: writePEM(t, "CERTIFICATE", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		keyFile:         writePEM(t, "RSA PRIVATE KEY", pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})),
	}
}

func writePEM(t *testing.T, blockType string, encoded []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), strings.ToLower(strings.ReplaceAll(blockType, " ", "-"))+".pem")
	require.NoError(t, os.WriteFile(path, encoded, 0o600))
	return path
}

func (a testCertificateAuthority) certPool(t *testing.T) *x509.CertPool {
	t.Helper()
	pool := x509.NewCertPool()
	pool.AddCert(a.certificate)
	return pool
}

type testCertificate struct {
	certificate     *x509.Certificate
	certificateFile string
	keyFile         string
}

func TestClientProductionAndLocalURLValidation(t *testing.T) {
	_, err := openapiworker.NewClient(openapiworker.ClientConfig{
		BaseURL: "https://worker.example.test",
	})
	requireErrorCode(t, err, openapiworker.ErrorCodeWorkerUnavailable)

	_, err = openapiworker.NewClient(openapiworker.ClientConfig{
		BaseURL: "http://example.test", DevMode: true,
	})
	requireErrorCode(t, err, openapiworker.ErrorCodeWorkerUnavailable)
}

func TestClientMTLSSuccessAndAuthenticationNegatives(t *testing.T) {
	authority, authorityFile := writeCA(t)
	clientCertificate := writeLeafCertificate(t, authority, pkix.Name{CommonName: "obot"}, x509.ExtKeyUsageClientAuth, nil, nil, func() time.Time { return time.Now().Add(time.Hour) })
	_, err := clientCertificate.certificate.Verify(x509.VerifyOptions{Roots: authority.certPool(t), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}})
	require.NoError(t, err)
	serverCertificate := writeLeafCertificate(t, authority, pkix.Name{CommonName: "worker"}, x509.ExtKeyUsageServerAuth, []string{"localhost"}, []net.IP{net.IPv4(127, 0, 0, 1)}, func() time.Time { return time.Now().Add(time.Hour) })
	_, wrongAuthorityFile := writeCA(t)
	expiredCertificate := writeLeafCertificate(t, authority, pkix.Name{CommonName: "expired-worker"}, x509.ExtKeyUsageServerAuth, []string{"localhost"}, []net.IP{net.IPv4(127, 0, 0, 1)}, func() time.Time { return time.Now().Add(-2 * time.Hour) })
	mismatchedCertificate := writeLeafCertificate(t, authority, pkix.Name{CommonName: "other-worker"}, x509.ExtKeyUsageServerAuth, []string{"other.invalid"}, nil, func() time.Time { return time.Now().Add(time.Hour) })

	startWorker := func(serverCertificate testCertificate) *httptest.Server {
		certificate, err := tls.LoadX509KeyPair(serverCertificate.certificateFile, serverCertificate.keyFile)
		require.NoError(t, err)
		worker := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		}))
		worker.TLS = &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{certificate},
			ClientCAs:    authority.certPool(t),
			ClientAuth:   tls.RequireAndVerifyClientCert,
		}
		worker.StartTLS()
		return worker
	}

	worker := startWorker(serverCertificate)
	defer worker.Close()
	client, err := openapiworker.NewClient(openapiworker.ClientConfig{
		BaseURL:        worker.URL,
		ServerCAFile:   authorityFile,
		ClientCertFile: clientCertificate.certificateFile,
		ClientKeyFile:  clientCertificate.keyFile,
	})
	require.NoError(t, err)
	defer client.Close()
	require.NoError(t, client.Ready(t.Context()))

	wrongCA, err := openapiworker.NewClient(openapiworker.ClientConfig{
		BaseURL:        worker.URL,
		ServerCAFile:   wrongAuthorityFile,
		ClientCertFile: clientCertificate.certificateFile,
		ClientKeyFile:  clientCertificate.keyFile,
	})
	require.NoError(t, err)
	defer wrongCA.Close()
	requireErrorCode(t, wrongCA.Ready(t.Context()), openapiworker.ErrorCodeWorkerUnavailable)

	_, err = openapiworker.NewClient(openapiworker.ClientConfig{
		BaseURL:      worker.URL,
		ServerCAFile: authorityFile,
	})
	requireErrorCode(t, err, openapiworker.ErrorCodeWorkerUnavailable)

	expiredWorker := startWorker(expiredCertificate)
	defer expiredWorker.Close()
	expiredClient, err := openapiworker.NewClient(openapiworker.ClientConfig{
		BaseURL:        expiredWorker.URL,
		ServerCAFile:   authorityFile,
		ClientCertFile: clientCertificate.certificateFile,
		ClientKeyFile:  clientCertificate.keyFile,
	})
	require.NoError(t, err)
	defer expiredClient.Close()
	requireErrorCode(t, expiredClient.Ready(t.Context()), openapiworker.ErrorCodeWorkerUnavailable)

	mismatchedWorker := startWorker(mismatchedCertificate)
	defer mismatchedWorker.Close()
	mismatchedClient, err := openapiworker.NewClient(openapiworker.ClientConfig{
		BaseURL:        mismatchedWorker.URL,
		ServerCAFile:   authorityFile,
		ClientCertFile: clientCertificate.certificateFile,
		ClientKeyFile:  clientCertificate.keyFile,
	})
	require.NoError(t, err)
	defer mismatchedClient.Close()
	requireErrorCode(t, mismatchedClient.Ready(t.Context()), openapiworker.ErrorCodeWorkerUnavailable)
}

func TestClientDoesNotFollowRedirects(t *testing.T) {
	var redirects atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/readyz" {
			redirects.Add(1)
			w.Header().Set("Location", "/other")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client, err := openapiworker.NewClient(openapiworker.ClientConfig{BaseURL: server.URL, DevMode: true})
	require.NoError(t, err)
	defer client.Close()
	requireErrorCode(t, client.Ready(t.Context()), openapiworker.ErrorCodeWorkerUnavailable)
	require.EqualValues(t, 1, redirects.Load())
}

func TestEnsureSnapshotReadySendsOneSnapshotRetry(t *testing.T) {
	var (
		describes  atomic.Int64
		ensures    atomic.Int64
		requestIDs = map[string]bool{}
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/internal/openapi/v1/describe":
			describes.Add(1)
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "application/json", r.Header.Get("Accept"))
			require.Equal(t, "Bearer second-factor", r.Header.Get("X-Obot-Worker-Authentication"))
			require.Empty(t, r.Header.Get("Authorization"))
			var request openapiworker.DescribeRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			requestIDs[request.RequestID] = true
			if describes.Load() == 1 && len(request.Snapshot) == 0 {
				writeWorkerError(w, http.StatusConflict, openapiworker.ErrorCodeSnapshotRequired)
				return
			}
			require.Len(t, requestIDs, 1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(openapiworker.DescribeResponse{
				Kind: "describe", Status: "ready", ProtocolVersion: openapiworker.ProtocolVersion,
				ExecutionDigest: request.ExecutionDigest, TotalOperations: 1, Complete: true,
			})
		case "/ensure":
			ensures.Add(1)
			http.NotFound(w, r)
		default:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		}
	}))
	defer server.Close()

	client, err := openapiworker.NewClient(openapiworker.ClientConfig{
		BaseURL: server.URL, DevMode: true, BearerToken: "second-factor",
	})
	require.NoError(t, err)
	defer client.Close()

	response, err := client.EnsureSnapshotReady(t.Context(), openapiworker.SnapshotEnvelope{
		ProtocolVersion: openapiworker.ProtocolVersion, SchemaBase64: "c2NoZW1h",
		ManifestBase64: base64.StdEncoding.EncodeToString([]byte(`{}`)),
		PolicyBase64:   base64.StdEncoding.EncodeToString([]byte(`{}`)),
		SchemaDigest:   "schema", ExecutionDigest: "execution",
	})
	require.NoError(t, err)
	require.True(t, response.Complete)
	require.EqualValues(t, 2, describes.Load())
	require.Zero(t, ensures.Load())
}

func TestEnsureSnapshotReadyFollowsDescribePagination(t *testing.T) {
	var cursors []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request openapiworker.DescribeRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		cursors = append(cursors, request.Cursor)
		nextCursor := 1
		complete := false
		if request.Cursor == 1 {
			nextCursor = 0
			complete = true
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openapiworker.DescribeResponse{
			Kind: "describe", Status: "ready", ProtocolVersion: openapiworker.ProtocolVersion,
			ExecutionDigest: request.ExecutionDigest, Operations: []openapiworker.WorkerOperation{{
				OperationKey: fmt.Sprintf("GET /items/%d", request.Cursor), LegacyToolName: "getItem",
				Method: http.MethodGet, PathTemplate: fmt.Sprintf("/items/%d", request.Cursor),
			}},
			NextCursor: &nextCursor, TotalOperations: 2, Complete: complete,
		})
	}))
	defer server.Close()

	client, err := openapiworker.NewClient(openapiworker.ClientConfig{BaseURL: server.URL, DevMode: true})
	require.NoError(t, err)
	defer client.Close()
	response, err := client.EnsureSnapshotReady(t.Context(), openapiworker.SnapshotEnvelope{
		ProtocolVersion: openapiworker.ProtocolVersion, ExecutionDigest: "execution",
	})
	require.NoError(t, err)
	require.Equal(t, []int{0, 1}, cursors)
	require.True(t, response.Complete)
	require.Len(t, response.Operations, 2)
}

func TestExecuteDoesNotRetrySnapshotRequired(t *testing.T) {
	var executes atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/internal/openapi/v1/execute", r.URL.Path)
		executes.Add(1)
		writeWorkerError(w, http.StatusConflict, openapiworker.ErrorCodeSnapshotRequired)
	}))
	defer server.Close()

	client, err := openapiworker.NewClient(openapiworker.ClientConfig{BaseURL: server.URL, DevMode: true})
	require.NoError(t, err)
	defer client.Close()

	_, err = client.Execute(t.Context(), openapiworker.ExecuteRequest{
		ProtocolVersion: openapiworker.ProtocolVersion,
		RequestID:       "request",
		ExecutionDigest: "execution",
		OperationKey:    "GET /items",
		DeadlineUnixMs:  time.Now().Add(time.Second).UnixMilli(),
	})
	requireErrorCode(t, err, openapiworker.ErrorCodeSnapshotRequired)
	require.EqualValues(t, 1, executes.Load())
}

func writeWorkerError(w http.ResponseWriter, status int, code openapiworker.ErrorCode) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(openapiworker.Error{Code: code, Message: string(code)})
}
