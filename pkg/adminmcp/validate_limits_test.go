package adminmcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/openapi"
	"github.com/obot-platform/obot/pkg/storage"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type validationImportFunc func(context.Context, types.OpenAPIRuntimeConfig) (*openapi.Result, error)

type validationLookupStorage struct {
	storage.Client
	started chan context.Context
	release chan struct{}
}

func (f validationImportFunc) Import(ctx context.Context, config types.OpenAPIRuntimeConfig) (*openapi.Result, error) {
	return f(ctx, config)
}

func openAPIValidationBatch(t *testing.T, count int) []CatalogFile {
	t.Helper()
	entries := make([]types.MCPServerCatalogEntryManifest, count)
	for i := range entries {
		entries[i] = types.MCPServerCatalogEntryManifest{
			Name:     fmt.Sprintf("API %d", i),
			EntryKey: fmt.Sprintf("api-%d", i),
			Runtime:  types.RuntimeOpenAPI,
			OpenAPIConfig: &types.OpenAPIRuntimeConfig{
				Source: types.OpenAPISource{URL: fmt.Sprintf("https://8.8.8.8/%d", i)},
			},
		}
	}
	data, err := json.Marshal(entries)
	require.NoError(t, err)
	return []CatalogFile{{Path: "apis.json", Content: string(data)}}
}

func TestValidationImportBudgets(t *testing.T) {
	tests := []struct {
		name      string
		count     int
		schema    string
		importErr error
		wantCalls int
		wantError string
	}{
		{
			name:      "successful imports count toward limit",
			count:     maxValidationImports + 2,
			schema:    validationOpenAPISchema,
			wantCalls: maxValidationImports,
			wantError: "at most 10 OpenAPI sources",
		},
		{
			name:      "failed imports count toward limit",
			count:     maxValidationImports + 2,
			importErr: errors.New("unavailable source"),
			wantCalls: maxValidationImports,
			wantError: "at most 10 OpenAPI sources",
		},
		{
			name:      "aggregate snapshots bounded before retention",
			count:     maxValidationImports,
			schema:    strings.TrimSuffix(validationOpenAPISchema, "}") + `,"x-padding":"` + strings.Repeat("x", openapi.MaxSchemaBytes-len(validationOpenAPISchema)-15) + `"}`,
			wantCalls: 9,
			wantError: "8 MiB validation budget",
		},
		{
			name:      "items within one file are bounded before import",
			count:     maxValidationItems + 1,
			wantCalls: 0,
			wantError: "at most 200 catalog items",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			validator := newTestValidator()
			validator.openAPIImporter = validationImportFunc(func(ctx context.Context, _ types.OpenAPIRuntimeConfig) (*openapi.Result, error) {
				calls++
				deadline, ok := ctx.Deadline()
				require.True(t, ok)
				require.LessOrEqual(t, time.Until(deadline), catalogValidationTimeout)
				return &openapi.Result{Schema: json.RawMessage(tt.schema)}, tt.importErr
			})
			_, err := validator.validate(t.Context(), openAPIValidationBatch(t, tt.count))
			require.ErrorContains(t, err, tt.wantError)
			require.Equal(t, tt.wantCalls, calls)
			if strings.Contains(tt.wantError, "MiB") {
				retained := 0
				for _, entry := range validator.entries {
					if entry.OpenAPIConfig != nil && entry.OpenAPIConfig.Schema != nil {
						retained += len(entry.OpenAPIConfig.Schema.Raw)
					}
				}
				require.LessOrEqual(t, retained, maxValidationSchemaBytes)
				require.Greater(t, retained, 7<<20)
			}
		})
	}
}

func (s *validationLookupStorage) Get(ctx context.Context, _ kclient.ObjectKey, _ kclient.Object, _ ...kclient.GetOption) error {
	s.started <- ctx
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.release:
		return context.Canceled
	}
}

func TestValidationDeadlineReachesAPILookups(t *testing.T) {
	store := &validationLookupStorage{
		started: make(chan context.Context, 1),
		release: make(chan struct{}),
	}
	defer close(store.release)
	tool := &tools{
		req: api.Context{
			Request: httptest.NewRequest(http.MethodPost, "/", nil),
			Storage: store,
		},
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, _, err := tool.validateCatalogFiles(ctx, nil, validateCatalogFilesInput{
			Files: []CatalogFile{{Path: "catalog.yaml", Content: "[]"}},
		})
		finished <- err
	}()
	select {
	case lookup := <-store.started:
		deadline, ok := lookup.Deadline()
		require.True(t, ok)
		require.LessOrEqual(t, time.Until(deadline), catalogValidationTimeout)
	case <-time.After(5 * time.Second):
		t.Fatal("catalog lookup did not start")
	}
	select {
	case err := <-finished:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(5 * time.Second):
		t.Fatal("API lookup ignored validation deadline")
	}
	// The bounded call must not mutate its caller's carrier request.
	_, hasDeadline := tool.req.Context().Deadline()
	require.False(t, hasDeadline)
}

func TestValidationDeadlineStopsImports(t *testing.T) {
	validator := newTestValidator()
	calls := 0
	validator.openAPIImporter = validationImportFunc(func(ctx context.Context, _ types.OpenAPIRuntimeConfig) (*openapi.Result, error) {
		calls++
		<-ctx.Done()
		return nil, ctx.Err()
	})
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	_, err := validator.validate(ctx, openAPIValidationBatch(t, 2))
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, 1, calls)
}

func TestConfigurationValidationSharesRepeatedSnapshots(t *testing.T) {
	schema := &types.OpenAPISchema{Raw: make(json.RawMessage, openapi.MaxSchemaBytes)}
	manifest := types.VMCPManifest{Components: make([]types.VMCPComponent, 64)}
	for i := range manifest.Components {
		manifest.Components[i] = types.VMCPComponent{
			ID: fmt.Sprintf("api-%d", i),
			CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
				OpenAPIConfig: &types.OpenAPIRuntimeConfig{Schema: schema},
				Config: []types.MCPConfig{{
					Key:      "TOKEN",
					Required: true,
				}},
			}},
			Configuration: []types.VMCPConfigurationPolicy{{
				Key:    "TOKEN",
				Policy: types.VMCPConfigurationPolicyFixed,
				Value:  "literal",
			}},
		}
	}
	prepared := copyConfigurationPolicies(manifest)
	values := vmcpconfig.ExtractStaticConfiguration(prepared, nil)
	require.Len(t, values, len(manifest.Components))
	for i := range prepared.Components {
		require.Same(t, schema, prepared.Components[i].CatalogEntry.Manifest.OpenAPIConfig.Schema)
		require.Empty(t, prepared.Components[i].Configuration[0].Value)
		require.Equal(t, "literal", manifest.Components[i].Configuration[0].Value)
	}
	validator := newTestValidator()
	require.NoError(t, validator.checkRequiredConfiguration(t.Context(), catalogItem{}, "bundle", manifest))
	require.Empty(t, validator.diagnostics)
}
