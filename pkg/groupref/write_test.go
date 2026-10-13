package groupref

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/i18n"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	oktaProviderName = "okta-auth-provider"
	// existingGroupID is a group of the SCIM provider, and pendingGroupID one that is being deleted because nothing
	// referenced it.
	existingGroupID = "okta/00g-team"
	pendingGroupID  = "okta/00g-old"
	pendingGroupID2 = "okta/00g-old-2"
	missingGroupID  = "okta/00g-missing"
)

// newWriteTestGateway returns a gateway with a SCIM connection for Okta, which has the groups existingGroupID and
// pendingGroupID and pendingGroupID2, the latter two marked for deletion.
func newWriteTestGateway(t *testing.T) *gclient.Client {
	t.Helper()

	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	if err != nil {
		t.Fatal(err)
	}
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	gateway := gclient.New(t.Context(), database, nil, nil, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { _ = gateway.Close() })

	conn, _, err := gateway.CreateSCIMConnection(t.Context(), gclient.CreateSCIMConnectionOptions{
		AuthProviderNamespace: namespace,
		AuthProviderName:      oktaProviderName,
		GroupIDPrefix:         "okta/",
		Origin:                gatewaytypes.SCIMConnectionOriginSCIMFirst,
	})
	if err != nil {
		t.Fatal(err)
	}
	db := services.DB.DB
	for _, id := range []string{existingGroupID, pendingGroupID, pendingGroupID2} {
		if err := db.Create(&gatewaytypes.Group{
			ID:                    id,
			AuthProviderName:      oktaProviderName,
			AuthProviderNamespace: namespace,
			Name:                  id,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{pendingGroupID, pendingGroupID2} {
		if err := db.Create(&gatewaytypes.SCIMPendingGroupDeletion{
			GroupID:      id,
			ConnectionID: conn.ID,
			RunID:        "run",
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	return gateway
}

func TestWriteNewGroupsLocalizedProblems(t *testing.T) {
	gateway := newWriteTestGateway(t)
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.AuthProvider{
		Name:      oktaProviderName,
		Namespace: namespace,
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: types.AuthProviderManifest{
				Name: "${local_provider_name}",
			},
		},
	}).Build()

	for _, tt := range []struct {
		locale   string
		provider string
		missing  string
		pending  string
	}{
		{
			locale:   "ja",
			provider: "ローカル",
			missing:  "先に ローカル からグループをプッシュ",
			pending:  "削除完了後に再試行",
		},
		{
			locale:   "ko",
			provider: "로컬",
			missing:  "먼저 로컬에서 그룹을 푸시",
			pending:  "삭제가 완료된 후 다시 시도",
		},
		{
			locale:   "zh-CN",
			provider: "本地",
			missing:  "请先从 本地 推送",
			pending:  "删除完成后重试",
		},
	} {
		t.Run(tt.locale, func(t *testing.T) {
			for _, groupIDs := range [][]string{
				{missingGroupID, pendingGroupID},
				{missingGroupID, missingGroupID + "-2", pendingGroupID, pendingGroupID2},
			} {
				err := WriteNewGroups(i18n.WithLocale(t.Context(), tt.locale), gateway, storage, groupIDs, func() error {
					t.Fatal("the write ran for a refused reference")
					return nil
				})
				httpErr, ok := errors.AsType[*types.ErrHTTP](err)
				if !ok || httpErr.Code != http.StatusBadRequest {
					t.Fatalf("WriteNewGroups() = %v, want a bad request", err)
				}
				for _, part := range append([]string{tt.provider, tt.missing, tt.pending, "; "}, groupIDs...) {
					if !strings.Contains(httpErr.Message, part) {
						t.Errorf("message %q does not contain %q", httpErr.Message, part)
					}
				}
			}
		})
	}
}

func TestWriteNewSubjects(t *testing.T) {
	gateway := newWriteTestGateway(t)
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.AuthProvider{
		Name:      oktaProviderName,
		Namespace: namespace,
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: types.AuthProviderManifest{
				Name: "Okta",
			},
		},
	}).Build()

	tests := []struct {
		name     string
		subjects []types.Subject
		previous []types.Subject
		// wantProblems are the parts of the refusal, which is a bad request. The write runs only without them.
		wantProblems []string
	}{
		{
			name:     "a new reference to a group of the provider",
			subjects: []types.Subject{groupSubject(existingGroupID)},
		},
		{
			name:         "a new reference to a group that the provider does not have",
			subjects:     []types.Subject{groupSubject(existingGroupID), groupSubject(missingGroupID)},
			wantProblems: []string{"no Okta group has the ID " + missingGroupID, "push the group from Okta first"},
		},
		{
			name:         "a new reference to a group that is being deleted",
			subjects:     []types.Subject{groupSubject(pendingGroupID)},
			wantProblems: []string{"the Okta group " + pendingGroupID + " is being deleted because nothing referenced it"},
		},
		{
			name:     "new references to a missing group and to one being deleted",
			subjects: []types.Subject{groupSubject(missingGroupID), groupSubject(pendingGroupID)},
			wantProblems: []string{
				"no Okta group has the ID " + missingGroupID,
				"the Okta group " + pendingGroupID + " is being deleted",
			},
		},
		{
			name:     "a reference to a missing group that was already there",
			subjects: []types.Subject{groupSubject(missingGroupID), groupSubject(existingGroupID)},
			previous: []types.Subject{groupSubject(missingGroupID)},
		},
		{
			name:     "a reference to a group of a provider without SCIM",
			subjects: []types.Subject{groupSubject("entra/engineering")},
		},
		{
			name: "subjects that are not groups",
			subjects: []types.Subject{
				{
					Type: types.SubjectTypeUser,
					ID:   missingGroupID,
				},
				{
					Type: types.SubjectTypeObotGroup,
					ID:   types.GroupAdmin,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var wrote bool
			err := WriteNewSubjects(t.Context(), gateway, storage, tt.subjects, tt.previous, func() error {
				wrote = true
				return nil
			})

			if len(tt.wantProblems) == 0 {
				if err != nil || !wrote {
					t.Fatalf("WriteNewSubjects() = %v, wrote = %v; want the write to run", err, wrote)
				}
				return
			}
			if wrote {
				t.Fatal("the write ran for a refused reference")
			}
			httpErr, ok := errors.AsType[*types.ErrHTTP](err)
			if !ok || httpErr.Code != http.StatusBadRequest {
				t.Fatalf("WriteNewSubjects() = %v, want a bad request", err)
			}
			for _, problem := range tt.wantProblems {
				if !strings.Contains(httpErr.Message, problem) {
					t.Errorf("message %q does not contain %q", httpErr.Message, problem)
				}
			}
		})
	}
}

func TestWriteNewGroupsNamesAProviderWithoutAnObjectByItsName(t *testing.T) {
	gateway := newWriteTestGateway(t)
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()

	err := WriteNewGroups(t.Context(), gateway, storage, []string{missingGroupID}, func() error {
		t.Fatal("the write ran for a refused reference")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "no "+oktaProviderName+" group has the ID "+missingGroupID) {
		t.Fatalf("WriteNewGroups() = %v, want a refusal that names the provider %q", err, oktaProviderName)
	}
}
