package handlers

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateServerAdmitsOnAttestation(t *testing.T) {
	manifest := types.MCPServerCatalogEntryManifest{
		Name:         "attested-entry",
		Runtime:      types.RuntimeRemote,
		RemoteConfig: &types.RemoteCatalogConfig{FixedURL: "https://mcp.example.com/mcp"},
		Attestation:  &types.MCPAttestationRef{URL: "https://example.com/statement.json"},
	}
	hash := utils.Digest(manifest)

	tests := []struct {
		name    string
		policy  mcp.AttestationPolicy
		status  *v1.MCPAttestationStatus
		wantErr string
	}{
		{
			name:    "not yet verified",
			wantErr: "has not been verified yet",
		},
		{
			name:    "verified for an earlier manifest",
			status:  &v1.MCPAttestationStatus{ManifestHash: "stale", Verified: true, SubjectMatch: true},
			wantErr: "earlier version of the entry",
		},
		{
			name:    "verification failed",
			status:  &v1.MCPAttestationStatus{ManifestHash: hash, Error: "fetch attestation: unexpected status 404 Not Found"},
			wantErr: "could not be verified",
		},
		{
			name:    "fails policy",
			policy:  mcp.AttestationPolicy{DenyFailIn: []string{"auth"}},
			status:  &v1.MCPAttestationStatus{ManifestHash: hash, Verified: true, SubjectMatch: true, FailedChecks: []string{"auth.unauthenticated_tools"}},
			wantErr: "denied category: auth.unauthenticated_tools",
		},
		{
			name:   "passes policy",
			policy: mcp.AttestationPolicy{MinScore: 80, DenyFailIn: []string{"auth"}},
			status: &v1.MCPAttestationStatus{ManifestHash: hash, Verified: true, SubjectMatch: true, Score: 91, FailedChecks: []string{"protocol.origin"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := newCreateServerSecretBindingTestHandler()
			handler.attestationPolicy = tt.policy
			entry := v1.MCPServerCatalogEntry{
				Name:      "entry-1",
				Namespace: system.DefaultNamespace,
				Spec:      v1.MCPServerCatalogEntrySpec{MCPCatalogName: "catalog-1", Manifest: manifest},
				Status:    v1.MCPServerCatalogEntryStatus{Attestation: tt.status},
			}
			storage := newFakeStorage(t, &v1.MCPCatalog{Name: "catalog-1", Namespace: system.DefaultNamespace}, &entry)

			err := handler.CreateServer(newCreateServerSecretBindingRequest(t, storage, nil, "catalog-1", "", types.MCPServer{
				CatalogEntryID: "entry-1",
			}))

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				var servers v1.MCPServerList
				require.NoError(t, storage.List(t.Context(), &servers))
				assert.Empty(t, servers.Items)
				return
			}
			// Admission is the first gate after access checks; anything past it is not
			// about the attestation.
			if err != nil {
				assert.NotContains(t, err.Error(), "attestation")
			}
		})
	}
}
