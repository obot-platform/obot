package license

import (
	"errors"
	"net/http"
	"testing"

	keygen "github.com/keygen-sh/keygen-go/v3"
	"github.com/obot-platform/obot/apiclient/types"
)

func TestMissingAndRequire(t *testing.T) {
	provider := &KeygenProvider{
		entitlements: map[keygen.EntitlementCode]struct{}{
			"ENTITLED": {},
		},
	}

	missing := provider.MissingEntitlements([]string{"ENTITLED", "MISSING"})
	if len(missing) != 1 || missing[0] != "MISSING" {
		t.Fatalf("Missing() = %v, want [MISSING]", missing)
	}

	if err := provider.RequireEntitlements([]string{"ENTITLED"}); err != nil {
		t.Fatalf("Require() error = %v, want nil", err)
	}

	err := provider.RequireEntitlements([]string{"MISSING"})
	var httpErr *types.ErrHTTP
	if !errors.As(err, &httpErr) {
		t.Fatalf("Require() error = %T, want *types.ErrHTTP", err)
	}
	if httpErr.Code != http.StatusPaymentRequired {
		t.Fatalf("Require() status = %d, want %d", httpErr.Code, http.StatusPaymentRequired)
	}
}
