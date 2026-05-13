package license

import (
	"errors"
	"net/http"
	"testing"

	keygen "github.com/keygen-sh/keygen-go/v3"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func TestMissingAndRequire(t *testing.T) {
	provider := &KeygenProvider{
		entitlements: map[keygen.EntitlementCode]struct{}{
			"ENTITLED": {},
		},
	}

	missing := provider.Missing([]string{"ENTITLED", "MISSING"})
	if len(missing) != 1 || missing[0] != "MISSING" {
		t.Fatalf("Missing() = %v, want [MISSING]", missing)
	}

	if err := provider.Require([]string{"ENTITLED"}); err != nil {
		t.Fatalf("Require() error = %v, want nil", err)
	}

	err := provider.Require([]string{"MISSING"})
	var httpErr *types.ErrHTTP
	if !errors.As(err, &httpErr) {
		t.Fatalf("Require() error = %T, want *types.ErrHTTP", err)
	}
	if httpErr.Code != http.StatusPaymentRequired {
		t.Fatalf("Require() status = %d, want %d", httpErr.Code, http.StatusPaymentRequired)
	}
}

func TestRequireForProvider(t *testing.T) {
	provider := &KeygenProvider{
		entitlements: map[keygen.EntitlementCode]struct{}{
			"AVAILABLE": {},
		},
	}

	ref := toolReferenceWithProviderMeta(`{
		"requiredEntitlements": ["AVAILABLE", "MISSING"],
		"envVars": [{"name":"API_KEY"}]
	}`)

	err := provider.RequireForProvider(ref)
	var httpErr *types.ErrHTTP
	if !errors.As(err, &httpErr) {
		t.Fatalf("RequireForProvider() error = %T, want *types.ErrHTTP", err)
	}
	if httpErr.Code != http.StatusPaymentRequired {
		t.Fatalf("RequireForProvider() status = %d, want %d", httpErr.Code, http.StatusPaymentRequired)
	}

	ref = toolReferenceWithProviderMeta(`{"requiredEntitlements": ["AVAILABLE"]}`)
	if err := provider.RequireForProvider(ref); err != nil {
		t.Fatalf("RequireForProvider() error = %v, want nil", err)
	}
}

func TestMetaForProvider(t *testing.T) {
	ref := toolReferenceWithProviderMeta(`{
		"requiredEntitlements": ["ENTITLEMENT"],
		"envVars": [{"name":"TOKEN"}]
	}`)

	meta, err := MetaForProvider(ref)
	if err != nil {
		t.Fatalf("MetaForProvider() error = %v", err)
	}
	if len(meta.RequiredEntitlements) != 1 || meta.RequiredEntitlements[0] != "ENTITLEMENT" {
		t.Fatalf("RequiredEntitlements = %v, want [ENTITLEMENT]", meta.RequiredEntitlements)
	}
	if len(meta.EnvVars) != 1 || meta.EnvVars[0].Name != "TOKEN" {
		t.Fatalf("EnvVars = %v, want TOKEN", meta.EnvVars)
	}

	ref = toolReferenceWithProviderMeta(`{`)
	if _, err := MetaForProvider(ref); err == nil {
		t.Fatal("MetaForProvider() error = nil, want invalid JSON error")
	}

	meta, err = MetaForProvider(v1.ToolReference{})
	if err != nil {
		t.Fatalf("MetaForProvider() with no tool error = %v", err)
	}
	if len(meta.RequiredEntitlements) != 0 || len(meta.EnvVars) != 0 {
		t.Fatalf("MetaForProvider() with no tool = %+v, want empty meta", meta)
	}
}

func toolReferenceWithProviderMeta(providerMeta string) v1.ToolReference {
	return v1.ToolReference{
		Status: v1.ToolReferenceStatus{
			Tool: &v1.ToolShortDescription{
				Metadata: map[string]string{
					"providerMeta": providerMeta,
				},
			},
		},
	}
}
