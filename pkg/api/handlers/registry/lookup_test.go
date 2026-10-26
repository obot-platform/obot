package registry

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func TestRegistryLookupErrors(t *testing.T) {
	storageFailure := errors.New("storage unavailable")
	serviceUnavailable := apierrors.NewServiceUnavailable("storage unavailable")
	for _, tc := range []struct {
		name    string
		id      string
		getErr  error
		listErr error
		wantErr error
	}{
		{
			name: "missing canonical ID",
			id:   "vmcp1missing",
		},
		{
			name: "missing alias",
			id:   "missing-alias",
		},
		{
			name:    "canonical lookup failure",
			id:      "vmcp1missing",
			getErr:  storageFailure,
			wantErr: storageFailure,
		},
		{
			name:    "alias lookup failure",
			id:      "missing-alias",
			listErr: storageFailure,
			wantErr: storageFailure,
		},
		{
			name:    "service unavailable",
			id:      "vmcp1missing",
			getErr:  serviceUnavailable,
			wantErr: serviceUnavailable,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			storage := newRegistryTestStorage()
			storage.getErr = tc.getErr
			storage.listErr = tc.listErr
			h := NewHandler("https://obot.example.com")
			for name, handler := range map[string]api.HandlerFunc{
				"versions": h.ListServerVersions,
				"latest":   h.GetServerVersion,
			} {
				t.Run(name, func(t *testing.T) {
					req := httptest.NewRequest(http.MethodGet, registryDetailPath(tc.id)+"/versions/latest", nil)
					req.SetPathValue("serverName", "com.example.obot/"+tc.id)
					req.SetPathValue("version", "latest")
					err := handler(api.Context{
						Request:        req,
						ResponseWriter: httptest.NewRecorder(),
						Storage:        storage,
					})
					if tc.wantErr != nil {
						if !errors.Is(err, tc.wantErr) {
							t.Fatalf("lookup error = %v, want %v", err, tc.wantErr)
						}
						return
					}
					var httpErr *types.ErrHTTP
					if !errors.As(err, &httpErr) || httpErr.Code != http.StatusNotFound {
						t.Fatalf("missing resource error = %v, want HTTP 404", err)
					}
				})
			}
		})
	}
}
