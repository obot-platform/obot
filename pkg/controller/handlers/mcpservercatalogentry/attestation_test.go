package mcpservercatalogentry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/utils"
	"github.com/sebastienrousseau/scout-reporting/attestation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	attestedEndpoint = "https://mcp.example.com/mcp"
)

// attestationServer serves one statement and counts the fetches, so "the controller did not
// go back to the network" is asserted rather than assumed.
type attestationServer struct {
	*httptest.Server
	fetches atomic.Int32
}

func newAttestationServer(t *testing.T) *attestationServer {
	t.Helper()
	target := attestation.Target{Transport: "http", Endpoint: attestedEndpoint}
	statement := attestation.Statement{
		Type:          attestation.StatementType,
		PredicateType: attestation.PredicateType,
		Subject:       []attestation.Subject{attestation.SubjectFor(target)},
		Predicate: attestation.Evaluation{
			SubjectKind:   attestation.SubjectKindDescriptor,
			Target:        target,
			JudgedAgainst: attestation.Basis{SpecRevision: "2026-07-28", Rubric: "1", CheckInventory: "1"},
			Instrument:    attestation.Instrument{Name: "scout", Version: "0.0.4", SchemaVersion: 1},
			RanAt:         time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC),
			Took:          "3.2s",
			Verdicts:      []attestation.Verdict{{ID: "auth.unauthenticated_tools", Phase: "auth", Status: "pass"}},
			Counts:        attestation.Counts{Pass: 1},
			Score:         &attestation.Score{Total: 100, Grade: "A", Assessed: 6, Of: 6},
		},
	}
	body, err := json.Marshal(statement)
	require.NoError(t, err)

	s := &attestationServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		s.fetches.Add(1)
		_, _ = w.Write(body)
	}))
	t.Cleanup(s.Close)
	return s
}

func attestedEntry(statementURL, fixedURL string) *v1.MCPServerCatalogEntry {
	return newMCPServerCatalogEntry("attested", types.MCPServerCatalogEntryManifest{
		Name:         "Attested",
		Runtime:      types.RuntimeRemote,
		RemoteConfig: &types.RemoteCatalogConfig{FixedURL: fixedURL},
		Attestation:  &types.MCPAttestationRef{URL: statementURL},
	})
}

func reconcileAttestation(t *testing.T, h *Handler, client kclient.WithWatch, entry *v1.MCPServerCatalogEntry) (*v1.MCPServerCatalogEntry, time.Duration) {
	t.Helper()
	resp := &router.ResponseWrapper{}
	require.NoError(t, h.ReconcileAttestation(router.Request{Ctx: t.Context(), Client: client, Object: entry}, resp))

	var stored v1.MCPServerCatalogEntry
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(entry), &stored))
	return &stored, resp.Delay
}

func TestReconcileAttestation(t *testing.T) {
	server := newAttestationServer(t)
	// The test server is on loopback, which the production client refuses on purpose.
	h := &Handler{attestationClient: server.Client()}

	entry := attestedEntry(server.URL+"/statement.json", attestedEndpoint)
	client := newFakeClient(entry)

	stored, delay := reconcileAttestation(t, h, client, entry)
	require.NotNil(t, stored.Status.Attestation)
	status := stored.Status.Attestation
	assert.True(t, status.Verified)
	assert.True(t, status.SubjectMatch)
	assert.Equal(t, utils.Digest(entry.Spec.Manifest), status.ManifestHash)
	assert.Equal(t, 100.0, status.Score)
	assert.Equal(t, "A", status.Grade)
	assert.Empty(t, status.Error)
	assert.Equal(t, attestationRecheckInterval, delay)
	assert.Equal(t, int32(1), server.fetches.Load())

	// A second pass for the same manifest costs no fetch and re-arms the recheck.
	_, delay = reconcileAttestation(t, h, client, stored)
	assert.Equal(t, int32(1), server.fetches.Load())
	assert.Greater(t, delay, time.Duration(0))
	assert.LessOrEqual(t, delay, attestationRecheckInterval)

	// Changing the entry's URL invalidates the result: the statement is about another server.
	stored.Spec.Manifest.RemoteConfig.FixedURL = "https://other.example.com/mcp"
	require.NoError(t, client.Update(t.Context(), stored))
	stored, delay = reconcileAttestation(t, h, client, stored)
	require.NotNil(t, stored.Status.Attestation)
	assert.True(t, stored.Status.Attestation.Verified)
	assert.False(t, stored.Status.Attestation.SubjectMatch)
	assert.Contains(t, stored.Status.Attestation.Error, "not https://other.example.com/mcp")
	assert.Equal(t, utils.Digest(stored.Spec.Manifest), stored.Status.Attestation.ManifestHash)
	assert.Equal(t, attestationRetryInterval, delay)
	assert.Equal(t, int32(2), server.fetches.Load())

	// A failed result is retried once its interval has passed, not on every event.
	stored.Status.Attestation.CheckedAt = &metav1.Time{Time: time.Now().Add(-attestationRetryInterval - time.Minute)}
	require.NoError(t, client.Status().Update(t.Context(), stored))
	_, _ = reconcileAttestation(t, h, client, stored)
	assert.Equal(t, int32(3), server.fetches.Load())

	// Removing the reference clears the status.
	stored.Spec.Manifest.Attestation = nil
	require.NoError(t, client.Update(t.Context(), stored))
	stored, _ = reconcileAttestation(t, h, client, stored)
	assert.Nil(t, stored.Status.Attestation)
	assert.Equal(t, int32(3), server.fetches.Load())
}

func TestReconcileAttestationRecordsFetchFailure(t *testing.T) {
	server := newAttestationServer(t)
	h := &Handler{attestationClient: server.Client()}
	entry := attestedEntry(server.URL+"/statement.json", attestedEndpoint)
	client := newFakeClient(entry)
	server.Close()

	stored, delay := reconcileAttestation(t, h, client, entry)
	require.NotNil(t, stored.Status.Attestation)
	assert.False(t, stored.Status.Attestation.Verified)
	assert.Contains(t, stored.Status.Attestation.Error, "fetch attestation")
	assert.Equal(t, attestationRetryInterval, delay)
}
