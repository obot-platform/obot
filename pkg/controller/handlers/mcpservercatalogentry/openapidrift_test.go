package mcpservercatalogentry

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/openapi"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fakeImporter struct {
	result  *openapi.Result
	err     error
	calls   int
	lastCtx context.Context
}

func (f *fakeImporter) Import(ctx context.Context, config types.OpenAPIRuntimeConfig) (*openapi.Result, error) {
	f.calls++
	f.lastCtx = ctx
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func openAPIEntry(url string, schema []byte) *v1.MCPServerCatalogEntry {
	entry := &v1.MCPServerCatalogEntry{}
	entry.Name = "entry-drift"
	entry.Spec.Manifest.Runtime = types.RuntimeOpenAPI
	config := &types.OpenAPIRuntimeConfig{
		Source:  types.OpenAPISource{URL: url},
		BaseURL: "https://api.example.test",
	}
	if schema != nil {
		config.Schema = &types.OpenAPISchema{Raw: json.RawMessage(schema)}
	}
	entry.Spec.Manifest.OpenAPIConfig = config
	return entry
}

func driftStoredSchema(t *testing.T) []byte {
	t.Helper()
	return []byte(`{"openapi":"3.1.0","info":{"title":"drift","version":"1"},"servers":[{"url":"https://api.example.test"}],"paths":{"/items":{"get":{"operationId":"listItems"}}}}`)
}

func newDriftFakeClient(objects ...kclient.Object) kclient.WithWatch {
	return fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(objects...).WithStatusSubresource(&v1.MCPServerCatalogEntry{}).Build()
}

func TestProbeOpenAPISourceDriftSkipsNonOpenAPIEntries(t *testing.T) {
	entry := &v1.MCPServerCatalogEntry{}
	entry.Name = "entry-remote"
	entry.Spec.Manifest.Runtime = types.RuntimeRemote
	importer := &fakeImporter{}
	handler := &Handler{OpenAPIImporter: importer}
	req := router.Request{Ctx: t.Context(), Client: newDriftFakeClient(entry), Object: entry}
	resp := router.Response(&fakeResponse{})
	require.NoError(t, handler.ProbeOpenAPISourceDrift(req, resp))
	assert.Zero(t, importer.calls, "non-OpenAPI entries must not be probed")
}

func TestProbeOpenAPISourceDriftSkipsInlineAndSnapshotless(t *testing.T) {
	// URL and Content both set is rejected upstream; probe must not fire for it either.
	inline := openAPIEntry("https://schema.example.test/openapi.json", driftStoredSchema(t))
	inline.Spec.Manifest.OpenAPIConfig.Source = types.OpenAPISource{Content: "{}"}
	noSnapshot := openAPIEntry("https://schema.example.test/openapi.json", nil)
	importer := &fakeImporter{}
	handler := &Handler{OpenAPIImporter: importer}
	for _, entry := range []*v1.MCPServerCatalogEntry{inline, noSnapshot} {
		req := router.Request{Ctx: t.Context(), Client: newDriftFakeClient(entry), Object: entry}
		resp := router.Response(&fakeResponse{})
		require.NoError(t, handler.ProbeOpenAPISourceDrift(req, resp))
	}
	assert.Zero(t, importer.calls, "inline sources and snapshotless entries must not be probed")
}

func TestProbeOpenAPISourceDriftReportsAndSchedules(t *testing.T) {
	stored := driftStoredSchema(t)
	// Source returns a changed schema: /items parameter added.
	changed := []byte(`{"openapi":"3.1.0","info":{"title":"drift","version":"1"},"servers":[{"url":"https://api.example.test"}],"paths":{"/items":{"get":{"operationId":"listItems","parameters":[{"name":"limit","in":"query"}]}}}}`)
	entry := openAPIEntry("https://schema.example.test/openapi.json", stored)
	storage := newDriftFakeClient(entry)
	importer := &fakeImporter{result: &openapi.Result{Schema: json.RawMessage(changed), BaseURL: "https://api.example.test"}}
	handler := &Handler{OpenAPIImporter: importer}
	resp := &fakeResponse{}
	req := router.Request{Ctx: t.Context(), Client: storage, Object: entry}
	require.NoError(t, handler.ProbeOpenAPISourceDrift(req, router.Response(resp)))

	assert.Equal(t, 1, importer.calls)
	assert.NotNil(t, importer.lastCtx, "probe must pass a bounded context")
	var updated v1.MCPServerCatalogEntry
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKey{Name: entry.Name}, &updated))
	drift := updated.Status.OpenAPISourceDrift
	require.NotNil(t, drift, "drift status must be persisted")
	assert.True(t, drift.Detected)
	assert.Equal(t, []string{"GET /items"}, drift.ChangedOperations)
	assert.Empty(t, drift.AddedOperations)
	assert.Empty(t, drift.RemovedOperations)
	assert.Empty(t, drift.ProbeError)
	assert.NotEmpty(t, drift.SnapshotDigest)
	assert.NotEmpty(t, drift.SourceDigest)
	assert.False(t, drift.NextProbeAt.Time.IsZero())
	assert.Greater(t, drift.NextProbeAt.Time, drift.LastProbedAt.Time)
	assert.Greater(t, resp.delay, time.Duration(0), "handler must reschedule itself")

	// Second reconcile before NextProbeAt must not re-probe.
	importer.calls = 0
	resp2 := &fakeResponse{}
	entryAfter := &updated
	require.NoError(t, handler.ProbeOpenAPISourceDrift(router.Request{Ctx: t.Context(), Client: storage, Object: entryAfter}, router.Response(resp2)))
	assert.Zero(t, importer.calls, "probe must respect NextProbeAt backoff")
}

func TestProbeOpenAPISourceDriftFetchFailureIsRecordedNotFatal(t *testing.T) {
	entry := openAPIEntry("https://schema.example.test/openapi.json", driftStoredSchema(t))
	storage := newDriftFakeClient(entry)
	importer := &fakeImporter{err: errors.New("schema fetch returned HTTP 503")}
	handler := &Handler{OpenAPIImporter: importer}
	resp := &fakeResponse{}
	require.NoError(t, handler.ProbeOpenAPISourceDrift(router.Request{Ctx: t.Context(), Client: storage, Object: entry}, router.Response(resp)))

	var updated v1.MCPServerCatalogEntry
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKey{Name: entry.Name}, &updated))
	drift := updated.Status.OpenAPISourceDrift
	require.NotNil(t, drift)
	assert.False(t, drift.Detected)
	assert.Contains(t, drift.ProbeError, "HTTP 503")
	assert.False(t, drift.NextProbeAt.Time.IsZero(), "failed probes must still reschedule")
}

func TestProbeOpenAPISourceDriftWithoutImporterRecordsConfigError(t *testing.T) {
	entry := openAPIEntry("https://schema.example.test/openapi.json", driftStoredSchema(t))
	storage := newDriftFakeClient(entry)
	handler := &Handler{}
	resp := &fakeResponse{}
	require.NoError(t, handler.ProbeOpenAPISourceDrift(router.Request{Ctx: t.Context(), Client: storage, Object: entry}, router.Response(resp)))
	var updated v1.MCPServerCatalogEntry
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKey{Name: entry.Name}, &updated))
	require.NotNil(t, updated.Status.OpenAPISourceDrift)
	assert.Equal(t, "drift probe is not configured", updated.Status.OpenAPISourceDrift.ProbeError)
}

// fakeResponse captures RetryAfter without importing nah internals.
type fakeResponse struct {
	delay time.Duration
	attrs map[string]any
}

func (f *fakeResponse) RetryAfter(delay time.Duration) { f.delay = delay }
func (f *fakeResponse) Attributes() map[string]any {
	if f.attrs == nil {
		f.attrs = map[string]any{}
	}
	return f.attrs
}
