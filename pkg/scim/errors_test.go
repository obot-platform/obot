package scim

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/scim/adapter"
)

// TestErrorHints writes each failure that has a hint as the handler does, once with Okta's hints and once with hints
// for every failure, and expects Okta's details to be the shared ones.
func TestErrorHints(t *testing.T) {
	okta, ok := adapter.Lookup("okta")
	if !ok {
		t.Fatal("the okta adapter is not registered")
	}
	hints := adapter.ErrorHints{
		Retry:                "Retry the task.",
		UserDelete:           "Set the deprovisioning action to Suspend, then retry the task.",
		ValuelessFilteredAdd: "Use the core connector.",
		ExternalIDChanged:    "Check the username setting.",
	}

	tests := []struct {
		name             string
		write            func(t *testing.T, w http.ResponseWriter)
		wantStatus       int
		wantDetail       string
		wantHintedDetail string
	}{
		{
			name: "a user DELETE",
			write: func(_ *testing.T, w http.ResponseWriter) {
				(&Handler{}).route(w, &request{
					Request: httptest.NewRequest(http.MethodDelete, PathPrefix+"Users/u1", nil),
					segment: []string{"Users", "u1"},
				})
			},
			wantStatus:       http.StatusNotImplemented,
			wantDetail:       "deleting users is not supported; deprovision a user by setting active to false",
			wantHintedDetail: "Set the deprovisioning action to Suspend, then retry the task.",
		},
		{
			name: "a request that takes too long",
			write: func(_ *testing.T, w http.ResponseWriter) {
				writeError(w, toError(fmt.Errorf("failed to update SCIM user: %w", context.DeadlineExceeded)))
			},
			wantStatus:       http.StatusServiceUnavailable,
			wantDetail:       "the request took too long; retry it",
			wantHintedDetail: "the request took too long; retry it. Retry the task.",
		},
		{
			name: "an unavailable endpoint",
			write: func(_ *testing.T, w http.ResponseWriter) {
				unavailable(w, "the auth provider of this SCIM connection is not configured")
			},
			wantStatus:       http.StatusServiceUnavailable,
			wantDetail:       "the auth provider of this SCIM connection is not configured",
			wantHintedDetail: "the auth provider of this SCIM connection is not configured. Retry the task.",
		},
		{
			name: "a filtered add without a value, as OneLogin's SCIM2 PATCH connector sends it",
			write: func(t *testing.T, w http.ResponseWriter) {
				t.Helper()
				writeError(w, toError(applyPatch(groupResourceSchema, map[string]any{
					"displayName": "group",
				}, decodeTestOperations(t, `[{"op":"add","path":"members[ value eq \"u1\" ]"}]`), adapter.PatchRules{})))
			},
			wantStatus:       http.StatusBadRequest,
			wantDetail:       "add requires a value",
			wantHintedDetail: "add requires a value. Use the core connector.",
		},
		{
			name: "a filtered add without a value of an attribute other than members",
			write: func(t *testing.T, w http.ResponseWriter) {
				t.Helper()
				writeError(w, toError(applyPatch(userResourceSchema, map[string]any{
					"userName": "user@example.com",
				}, decodeTestOperations(t, `[{"op":"add","path":"emails[type eq \"work\"]"}]`), adapter.PatchRules{})))
			},
			wantStatus:       http.StatusBadRequest,
			wantDetail:       "add requires a value",
			wantHintedDetail: "add requires a value",
		},
		{
			name: "an add without a value or a filter",
			write: func(t *testing.T, w http.ResponseWriter) {
				t.Helper()
				writeError(w, toError(applyPatch(groupResourceSchema, map[string]any{
					"displayName": "group",
				}, decodeTestOperations(t, `[{"op":"add","path":"members"}]`), adapter.PatchRules{})))
			},
			wantStatus:       http.StatusBadRequest,
			wantDetail:       "add requires a value",
			wantHintedDetail: "add requires a value",
		},
		{
			name: "a changed externalId",
			write: func(_ *testing.T, w http.ResponseWriter) {
				writeError(w, toError(fmt.Errorf("failed to update SCIM user: %w", &gclient.SCIMMutabilityError{
					Attribute: "externalId",
					Message:   "externalId identifies the user and cannot change",
				})))
			},
			wantStatus:       http.StatusBadRequest,
			wantDetail:       "externalId identifies the user and cannot change",
			wantHintedDetail: "externalId identifies the user and cannot change. Check the username setting.",
		},
		{
			name: "a change to another immutable attribute",
			write: func(_ *testing.T, w http.ResponseWriter) {
				writeError(w, toError(&gclient.SCIMMutabilityError{
					Message: "id cannot change",
				}))
			},
			wantStatus:       http.StatusBadRequest,
			wantDetail:       "id cannot change",
			wantHintedDetail: "id cannot change",
		},
		{
			name: "a failure without a hint",
			write: func(_ *testing.T, w http.ResponseWriter) {
				writeError(w, toError(&gclient.SCIMInvalidValueError{
					Message: "externalId is required",
				}))
			},
			wantStatus:       http.StatusBadRequest,
			wantDetail:       "externalId is required",
			wantHintedDetail: "externalId is required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, run := range []struct {
				hints      adapter.ErrorHints
				wantDetail string
			}{
				{
					hints:      okta.ErrorHints(),
					wantDetail: tt.wantDetail,
				},
				{
					hints:      hints,
					wantDetail: tt.wantHintedDetail,
				},
			} {
				w := httptest.NewRecorder()
				rec := &statusRecorder{
					ResponseWriter: w,
					hints:          run.hints,
				}
				tt.write(t, rec)

				var body errorResponse
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
					t.Fatalf("failed to decode the response: %v: %s", err, w.Body.String())
				}
				if w.Code != tt.wantStatus || body.Status != fmt.Sprint(tt.wantStatus) {
					t.Errorf("status = %d (%q), want %d", w.Code, body.Status, tt.wantStatus)
				}
				if body.Detail != run.wantDetail {
					t.Errorf("detail = %q, want %q", body.Detail, run.wantDetail)
				}
				// The activity log records the detail that was sent.
				if rec.err == nil || rec.err.Detail != run.wantDetail {
					t.Errorf("recorded error = %v, want detail %q", rec.err, run.wantDetail)
				}
			}
		})
	}
}

func TestErrorWithHintsLeavesTheErrorAlone(t *testing.T) {
	hints := adapter.ErrorHints{
		Retry:      "Retry the task.",
		UserDelete: "Set the deprovisioning action to Suspend.",
	}
	tests := []struct {
		name             string
		err              Error
		wantHintedDetail string
	}{
		{
			name: "an appended hint",
			err: Error{
				Status: http.StatusServiceUnavailable,
				Detail: "the request took too long; retry it",
				hint:   hintRetry,
			},
			wantHintedDetail: "the request took too long; retry it. Retry the task.",
		},
		{
			name: "a hint that replaces the detail",
			err: Error{
				Status: http.StatusNotImplemented,
				Detail: "deleting users is not supported; deprovision a user by setting active to false",
				hint:   hintUserDelete,
			},
			wantHintedDetail: "Set the deprovisioning action to Suspend.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.err
			hinted := err.withHints(hints)
			if err.Detail != tt.err.Detail {
				t.Fatalf("withHints changed the error's detail to %q", err.Detail)
			}
			if hinted.Detail != tt.wantHintedDetail {
				t.Fatalf("hinted detail = %q, want %q", hinted.Detail, tt.wantHintedDetail)
			}
		})
	}
}

// TestOktaErrorDetails checks that the details of the failures that have hints are the shared ones for an Okta
// connection, end to end.
func TestOktaErrorDetails(t *testing.T) {
	s := newSCIMTest(t)
	s.enable()

	detail := func(resp testResponse) any {
		return resp.body["detail"]
	}

	if got, want := detail(s.do(http.MethodDelete, "Users/anything", nil).expect(t, http.StatusNotImplemented)),
		"deleting users is not supported; deprovision a user by setting active to false"; got != want {
		t.Errorf("user DELETE detail = %q, want %q", got, want)
	}

	user := s.do(http.MethodPost, "Users", scimUser("user@example.com", "00u-user")).expect(t, http.StatusCreated)
	if got, want := detail(s.do(http.MethodPut, "Users/"+user.id(), scimUser("user@example.com", "00u-other")).expect(t, http.StatusBadRequest)),
		"externalId identifies the user and cannot change"; got != want {
		t.Errorf("changed externalId detail = %q, want %q", got, want)
	}

	group := s.do(http.MethodPost, "Groups", scimGroup("Team")).expect(t, http.StatusCreated)
	if got, want := detail(s.do(http.MethodPatch, "Groups/"+group.id(), patchOp(map[string]any{
		"op":   "add",
		"path": `members[value eq "` + user.id() + `"]`,
	})).expect(t, http.StatusBadRequest)), "add requires a value"; got != want {
		t.Errorf("valueless filtered add detail = %q, want %q", got, want)
	}

	s.env.name = "another-auth-provider"
	if got, want := detail(s.do(http.MethodGet, "Users", nil).expect(t, http.StatusServiceUnavailable)),
		"the auth provider of this SCIM connection is not configured"; got != want {
		t.Errorf("unconfigured auth provider detail = %q, want %q", got, want)
	}
}
