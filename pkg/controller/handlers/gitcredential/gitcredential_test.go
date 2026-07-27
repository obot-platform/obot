package gitcredential

import (
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSyncReferences(t *testing.T) {
	credential := &v1.GitCredential{
		ObjectMeta: metav1.ObjectMeta{Name: "gc1-test", Namespace: "default"},
	}
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		&v1.SkillRepository{
			ObjectMeta: metav1.ObjectMeta{Name: "skills", Namespace: "default"},
			Spec: v1.SkillRepositorySpec{
				DisplayName:     "Team Skills",
				GitCredentialID: credential.Name,
			},
		},
		&v1.MCPCatalog{
			ObjectMeta: metav1.ObjectMeta{Name: "catalog", Namespace: "default"},
			Spec: v1.MCPCatalogSpec{SourceURLGitCredentialIDs: map[string]string{
				"https://github.com/obot-platform/catalog": credential.Name,
			}},
		},
		&v1.SystemMCPCatalog{
			ObjectMeta: metav1.ObjectMeta{Name: "system-catalog", Namespace: "default"},
			Spec: v1.SystemMCPCatalogSpec{SourceURLGitCredentialIDs: map[string]string{
				"https://github.com/obot-platform/system-catalog": credential.Name,
			}},
		},
	).Build()

	err := (&Handler{}).SyncReferences(router.Request{
		Client:    storage,
		Ctx:       t.Context(),
		Object:    credential,
		Namespace: credential.Namespace,
		Name:      credential.Name,
	}, &router.ResponseWrapper{})
	require.NoError(t, err)
	assert.Equal(t, v1.GitCredentialReferences{
		SkillRepositories: []v1.GitCredentialReference{{ID: "skills", DisplayName: "Team Skills"}},
		MCPCatalogs: []v1.GitCredentialReference{{
			ID:          "catalog",
			DisplayName: "https://github.com/obot-platform/catalog",
		}},
		SystemMCPCatalogs: []v1.GitCredentialReference{{
			ID:          "system-catalog",
			DisplayName: "https://github.com/obot-platform/system-catalog",
		}},
	}, credential.Status.References)
}

func TestSyncReferencesClearsRemovedReferences(t *testing.T) {
	credential := &v1.GitCredential{
		ObjectMeta: metav1.ObjectMeta{Name: "gc1-test", Namespace: "default"},
		Status: v1.GitCredentialStatus{References: v1.GitCredentialReferences{
			SkillRepositories: []v1.GitCredentialReference{{ID: "removed"}},
		}},
	}
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()

	err := (&Handler{}).SyncReferences(router.Request{
		Client:    storage,
		Ctx:       t.Context(),
		Object:    credential,
		Namespace: credential.Namespace,
		Name:      credential.Name,
	}, &router.ResponseWrapper{})
	require.NoError(t, err)
	assert.Empty(t, credential.Status.References)
}
