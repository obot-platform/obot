package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/sebastienrousseau/scout-reporting/attestation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	attestedEndpoint = "https://mcp.example.com/mcp"
)

// validStatement is shaped like the one scout writes; the attestation module's own tests
// build theirs the same way.
func validStatement() *attestation.Statement {
	target := attestation.Target{Transport: "http", Endpoint: attestedEndpoint}
	return &attestation.Statement{
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
			Verdicts: []attestation.Verdict{
				{ID: "auth.unauthenticated_tools", Phase: "auth", Status: "pass"},
				{ID: "protocol.origin", Phase: "protocol", Status: "fail", Severity: "major", Evidence: []string{"req#12"}},
			},
			Counts: attestation.Counts{Pass: 1, Fail: 1},
			Score:  &attestation.Score{Total: 88, Grade: "B", Assessed: 6, Of: 6},
		},
	}
}

func serveStatement(t *testing.T, body []byte, code int) (*httptest.Server, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return server, server.URL + "/statement.json"
}

func TestVerifyAttestation(t *testing.T) {
	tests := []struct {
		name string
		// statement is served when set; raw otherwise.
		statement     func() *attestation.Statement
		raw           []byte
		code          int
		endpoint      string
		wantVerified  bool
		wantSubject   bool
		wantErrSubstr string
	}{
		{
			name:         "valid statement about the entry",
			statement:    validStatement,
			code:         http.StatusOK,
			endpoint:     attestedEndpoint,
			wantVerified: true,
			wantSubject:  true,
		},
		{
			name: "tampered subject digest",
			statement: func() *attestation.Statement {
				s := validStatement()
				s.Subject[0].Digest["sha256"] = strings.Repeat("0", 64)
				return s
			},
			code:          http.StatusOK,
			endpoint:      attestedEndpoint,
			wantErrSubstr: "does not cover the target",
		},
		{
			name:          "statement about a different server",
			statement:     validStatement,
			code:          http.StatusOK,
			endpoint:      "https://other.example.com/mcp",
			wantVerified:  true,
			wantErrSubstr: "not https://other.example.com/mcp",
		},
		{
			name:          "fetch error",
			raw:           []byte("gone"),
			code:          http.StatusNotFound,
			endpoint:      attestedEndpoint,
			wantErrSubstr: "unexpected status 404",
		},
		{
			name:          "not a statement",
			raw:           []byte(`{"hello":"world"}`),
			code:          http.StatusOK,
			endpoint:      attestedEndpoint,
			wantErrSubstr: "parse attestation",
		},
		{
			name:          "oversize body",
			raw:           []byte(strings.Repeat(" ", AttestationMaxBytes+1)),
			code:          http.StatusOK,
			endpoint:      attestedEndpoint,
			wantErrSubstr: "exceeds",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.raw
			if tt.statement != nil {
				var err error
				body, err = json.Marshal(tt.statement())
				require.NoError(t, err)
			}
			server, statementURL := serveStatement(t, body, tt.code)
			status := VerifyAttestation(t.Context(), server.Client(), statementURL, tt.endpoint)

			assert.Equal(t, tt.wantVerified, status.Verified, "verified")
			assert.Equal(t, tt.wantSubject, status.SubjectMatch, "subject match")
			assert.NotNil(t, status.CheckedAt)
			if tt.wantErrSubstr == "" {
				assert.Empty(t, status.Error)
				assert.Equal(t, 88.0, status.Score)
				assert.Equal(t, "B", status.Grade)
				assert.Equal(t, 1, status.FailCount)
				assert.Equal(t, []string{"protocol.origin"}, status.FailedChecks)
				assert.Equal(t, "scout 0.0.4", status.Instrument)
				require.NotNil(t, status.RanAt)
				assert.Equal(t, validStatement().Predicate.RanAt, status.RanAt.UTC())
			} else {
				assert.Contains(t, status.Error, tt.wantErrSubstr)
			}
		})
	}
}

func TestVerifyAttestationUnreachable(t *testing.T) {
	server, statementURL := serveStatement(t, nil, http.StatusOK)
	client := server.Client()
	server.Close()

	status := VerifyAttestation(t.Context(), client, statementURL, attestedEndpoint)
	assert.False(t, status.Verified)
	assert.Contains(t, status.Error, "fetch attestation")
}

func TestAttestationPolicyEvaluate(t *testing.T) {
	verified := v1.MCPAttestationStatus{
		Verified:     true,
		SubjectMatch: true,
		Score:        88,
		FailedChecks: []string{"protocol.origin"},
	}
	tests := []struct {
		name    string
		policy  AttestationPolicy
		status  v1.MCPAttestationStatus
		wantErr string
	}{
		{
			name:   "zero policy admits a verified statement",
			status: verified,
		},
		{
			name:    "unverified",
			status:  v1.MCPAttestationStatus{Error: "fetch attestation: boom"},
			wantErr: "could not be verified: fetch attestation: boom",
		},
		{
			name:    "subject mismatch",
			status:  v1.MCPAttestationStatus{Verified: true, Error: "attestation is about http x, not y"},
			wantErr: "does not cover the entry's URL",
		},
		{
			name:    "below minimum score",
			policy:  AttestationPolicy{MinScore: 90},
			status:  verified,
			wantErr: "score 88 is below the required minimum 90",
		},
		{
			name:   "at minimum score",
			policy: AttestationPolicy{MinScore: 88},
			status: verified,
		},
		{
			name:    "fail in denied category",
			policy:  AttestationPolicy{DenyFailIn: []string{"protocol"}},
			status:  verified,
			wantErr: "denied category: protocol.origin",
		},
		{
			name:   "fail in another category",
			policy: AttestationPolicy{DenyFailIn: []string{"auth"}},
			status: verified,
		},
		{
			name:    "no score with a minimum",
			policy:  AttestationPolicy{MinScore: 1},
			status:  v1.MCPAttestationStatus{Verified: true, SubjectMatch: true},
			wantErr: "score 0 is below",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.policy.Evaluate(tt.status)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestAttestationPolicyAdmit(t *testing.T) {
	entry := func(ref *types.MCPAttestationRef, status *v1.MCPAttestationStatus) v1.MCPServerCatalogEntry {
		return v1.MCPServerCatalogEntry{
			Spec:   v1.MCPServerCatalogEntrySpec{Manifest: types.MCPServerCatalogEntryManifest{Attestation: ref}},
			Status: v1.MCPServerCatalogEntryStatus{Attestation: status},
		}
	}
	ref := &types.MCPAttestationRef{URL: "https://example.com/statement.json"}
	current := &v1.MCPAttestationStatus{ManifestHash: "h1", Verified: true, SubjectMatch: true}

	tests := []struct {
		name    string
		entry   v1.MCPServerCatalogEntry
		wantErr string
	}{
		{
			name:  "no attestation reference",
			entry: entry(nil, nil),
		},
		{
			name:    "not yet verified",
			entry:   entry(ref, nil),
			wantErr: "has not been verified yet",
		},
		{
			name:    "stale verification",
			entry:   entry(ref, &v1.MCPAttestationStatus{ManifestHash: "h0", Verified: true, SubjectMatch: true}),
			wantErr: "earlier version",
		},
		{
			name:  "current verification",
			entry: entry(ref, current),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := AttestationPolicy{}.Admit(tt.entry, "h1")
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestNewAttestationPolicy(t *testing.T) {
	policy, err := NewAttestationPolicy(75, []string{" Auth", "protocol", "", "auth"})
	require.NoError(t, err)
	assert.Equal(t, AttestationPolicy{MinScore: 75, DenyFailIn: []string{"auth", "protocol"}}, policy)

	_, err = NewAttestationPolicy(101, nil)
	assert.Error(t, err)
	_, err = NewAttestationPolicy(-1, nil)
	assert.Error(t, err)
}

func TestValidateAttestationRef(t *testing.T) {
	remote := func(fixedURL string) types.MCPServerCatalogEntryManifest {
		return types.MCPServerCatalogEntryManifest{
			Runtime:      types.RuntimeRemote,
			RemoteConfig: &types.RemoteCatalogConfig{FixedURL: fixedURL},
			Attestation:  &types.MCPAttestationRef{URL: "https://example.com/statement.json"},
		}
	}
	tests := []struct {
		name     string
		manifest types.MCPServerCatalogEntryManifest
		wantErr  string
	}{
		{
			name:     "no reference",
			manifest: types.MCPServerCatalogEntryManifest{Runtime: types.RuntimeNPX},
		},
		{
			name:     "remote with fixed URL",
			manifest: remote(attestedEndpoint),
		},
		{
			name: "non-remote runtime",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime:     types.RuntimeNPX,
				Attestation: &types.MCPAttestationRef{URL: "https://example.com/statement.json"},
			},
			wantErr: "only supported for remote entries",
		},
		{
			name: "remote without fixed URL",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{Hostname: "example.com"},
				Attestation:  &types.MCPAttestationRef{URL: "https://example.com/statement.json"},
			},
			wantErr: "require remoteConfig.fixedURL",
		},
		{
			name:     "templated fixed URL",
			manifest: remote("https://${TENANT}.example.com/mcp"),
			wantErr:  "without template references",
		},
		{
			name: "http statement URL",
			manifest: func() types.MCPServerCatalogEntryManifest {
				m := remote(attestedEndpoint)
				m.Attestation.URL = "http://example.com/statement.json"
				return m
			}(),
			wantErr: "must be an https URL",
		},
		{
			name: "user information in statement URL",
			manifest: func() types.MCPServerCatalogEntryManifest {
				m := remote(attestedEndpoint)
				m.Attestation.URL = "https://user:pass@example.com/statement.json"
				return m
			}(),
			wantErr: "user information",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAttestationRef(tt.manifest)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
