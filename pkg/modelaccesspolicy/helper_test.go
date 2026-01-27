package modelaccesspolicy

import (
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
)

// Test authGroupSet function
func TestAuthGroupSet(t *testing.T) {
	tests := []struct {
		name     string
		user     kuser.Info
		expected map[string]struct{}
	}{
		{
			name: "user with multiple groups",
			user: &kuser.DefaultInfo{
				Extra: map[string][]string{
					"auth_provider_groups": {"group1", "group2", "group3"},
				},
			},
			expected: map[string]struct{}{
				"group1": {},
				"group2": {},
				"group3": {},
			},
		},
		{
			name: "user with single group",
			user: &kuser.DefaultInfo{
				Extra: map[string][]string{
					"auth_provider_groups": {"group1"},
				},
			},
			expected: map[string]struct{}{
				"group1": {},
			},
		},
		{
			name: "user with no groups",
			user: &kuser.DefaultInfo{
				Extra: map[string][]string{},
			},
			expected: map[string]struct{}{},
		},
		{
			name: "user with empty groups list",
			user: &kuser.DefaultInfo{
				Extra: map[string][]string{
					"auth_provider_groups": {},
				},
			},
			expected: map[string]struct{}{},
		},
		{
			name:     "user with nil extra",
			user:     &kuser.DefaultInfo{},
			expected: map[string]struct{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := authGroupSet(tt.user)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test mapSubjectIndexFunc function
func TestMapSubjectIndexFunc(t *testing.T) {
	tests := []struct {
		name        string
		subjectType types.SubjectType
		policy      *v1.ModelAccessPolicy
		expected    []string
		expectNil   bool
	}{
		{
			name:        "policy with matching user subjects",
			subjectType: types.SubjectTypeUser,
			policy: &v1.ModelAccessPolicy{
				Spec: v1.ModelAccessPolicySpec{
					Manifest: types.ModelAccessPolicyManifest{
						Subjects: []types.Subject{
							{Type: types.SubjectTypeUser, ID: "user1"},
							{Type: types.SubjectTypeUser, ID: "user2"},
							{Type: types.SubjectTypeGroup, ID: "group1"},
						},
					},
				},
			},
			expected: []string{"user1", "user2"},
		},
		{
			name:        "policy with matching group subjects",
			subjectType: types.SubjectTypeGroup,
			policy: &v1.ModelAccessPolicy{
				Spec: v1.ModelAccessPolicySpec{
					Manifest: types.ModelAccessPolicyManifest{
						Subjects: []types.Subject{
							{Type: types.SubjectTypeUser, ID: "user1"},
							{Type: types.SubjectTypeGroup, ID: "group1"},
							{Type: types.SubjectTypeGroup, ID: "group2"},
						},
					},
				},
			},
			expected: []string{"group1", "group2"},
		},
		{
			name:        "policy with selector subjects",
			subjectType: types.SubjectTypeSelector,
			policy: &v1.ModelAccessPolicy{
				Spec: v1.ModelAccessPolicySpec{
					Manifest: types.ModelAccessPolicyManifest{
						Subjects: []types.Subject{
							{Type: types.SubjectTypeSelector, ID: "*"},
							{Type: types.SubjectTypeUser, ID: "user1"},
						},
					},
				},
			},
			expected: []string{"*"},
		},
		{
			name:        "policy with no matching subjects",
			subjectType: types.SubjectTypeUser,
			policy: &v1.ModelAccessPolicy{
				Spec: v1.ModelAccessPolicySpec{
					Manifest: types.ModelAccessPolicyManifest{
						Subjects: []types.Subject{
							{Type: types.SubjectTypeGroup, ID: "group1"},
						},
					},
				},
			},
			expected: []string{},
		},
		{
			name:        "policy with empty subjects",
			subjectType: types.SubjectTypeUser,
			policy: &v1.ModelAccessPolicy{
				Spec: v1.ModelAccessPolicySpec{
					Manifest: types.ModelAccessPolicyManifest{
						Subjects: []types.Subject{},
					},
				},
			},
			expected: []string{},
		},
		{
			name:        "deleted policy",
			subjectType: types.SubjectTypeUser,
			policy: &v1.ModelAccessPolicy{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
				},
				Spec: v1.ModelAccessPolicySpec{
					Manifest: types.ModelAccessPolicyManifest{
						Subjects: []types.Subject{
							{Type: types.SubjectTypeUser, ID: "user1"},
						},
					},
				},
			},
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			indexFunc := mapSubjectIndexFunc(tt.subjectType)
			result, err := indexFunc(tt.policy)
			require.NoError(t, err)
			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// Test dmaModelIndexFunc function
func TestDmaModelIndexFunc(t *testing.T) {
	tests := []struct {
		name      string
		dma       *v1.DefaultModelAlias
		expected  []string
		expectNil bool
	}{
		{
			name: "valid llm alias",
			dma: &v1.DefaultModelAlias{
				ObjectMeta: metav1.ObjectMeta{
					Name: "llm",
				},
				Spec: v1.DefaultModelAliasSpec{
					Manifest: types.DefaultModelAliasManifest{
						Alias: "llm",
						Model: "m1-gpt-4o",
					},
				},
			},
			expected: []string{"llm/m1-gpt-4o"},
		},
		{
			name: "valid vision alias",
			dma: &v1.DefaultModelAlias{
				ObjectMeta: metav1.ObjectMeta{
					Name: "vision",
				},
				Spec: v1.DefaultModelAliasSpec{
					Manifest: types.DefaultModelAliasManifest{
						Alias: "vision",
						Model: "m1-vision-model",
					},
				},
			},
			expected: []string{"vision/m1-vision-model"},
		},
		{
			name: "deleted alias",
			dma: &v1.DefaultModelAlias{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "llm",
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
				},
				Spec: v1.DefaultModelAliasSpec{
					Manifest: types.DefaultModelAliasManifest{
						Alias: "llm",
						Model: "m1-gpt-4o",
					},
				},
			},
			expectNil: true,
		},
		{
			name: "invalid model ID (empty)",
			dma: &v1.DefaultModelAlias{
				ObjectMeta: metav1.ObjectMeta{
					Name: "llm",
				},
				Spec: v1.DefaultModelAliasSpec{
					Manifest: types.DefaultModelAliasManifest{
						Alias: "llm",
						Model: "",
					},
				},
			},
			expectNil: true,
		},
		{
			name: "invalid alias type (unknown)",
			dma: &v1.DefaultModelAlias{
				ObjectMeta: metav1.ObjectMeta{
					Name: "invalid",
				},
				Spec: v1.DefaultModelAliasSpec{
					Manifest: types.DefaultModelAliasManifest{
						Alias: "invalid",
						Model: "gpt-4o-2024-08-06",
					},
				},
			},
			expectNil: true,
		},
		{
			name: "alias name mismatch",
			dma: &v1.DefaultModelAlias{
				ObjectMeta: metav1.ObjectMeta{
					Name: "llm",
				},
				Spec: v1.DefaultModelAliasSpec{
					Manifest: types.DefaultModelAliasManifest{
						Alias: "vision",
						Model: "m1-gpt-4o",
					},
				},
			},
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := dmaModelIndexFunc(tt.dma)
			require.NoError(t, err)
			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// Mock indexer for testing Helper methods
type mockIndexer struct {
	data   map[string]map[string][]interface{}
	values []string
}

func newMockIndexer() *mockIndexer {
	return &mockIndexer{
		data: make(map[string]map[string][]interface{}),
	}
}

func (m *mockIndexer) Add(obj interface{}) error {
	return nil
}

func (m *mockIndexer) Update(obj interface{}) error {
	return nil
}

func (m *mockIndexer) Delete(obj interface{}) error {
	return nil
}

func (m *mockIndexer) List() []interface{} {
	return nil
}

func (m *mockIndexer) ListKeys() []string {
	return nil
}

func (m *mockIndexer) Get(obj interface{}) (item interface{}, exists bool, err error) {
	return nil, false, nil
}

func (m *mockIndexer) GetByKey(key string) (item interface{}, exists bool, err error) {
	return nil, false, nil
}

func (m *mockIndexer) Replace([]interface{}, string) error {
	return nil
}

func (m *mockIndexer) Resync() error {
	return nil
}

func (m *mockIndexer) Index(indexName string, obj interface{}) ([]interface{}, error) {
	return nil, nil
}

func (m *mockIndexer) IndexKeys(indexName, indexedValue string) ([]string, error) {
	return nil, nil
}

func (m *mockIndexer) ListIndexFuncValues(indexName string) []string {
	return m.values
}

func (m *mockIndexer) ByIndex(indexName, indexedValue string) ([]interface{}, error) {
	if idx, ok := m.data[indexName]; ok {
		if objs, ok := idx[indexedValue]; ok {
			return objs, nil
		}
	}
	return []interface{}{}, nil
}

func (m *mockIndexer) GetIndexers() gocache.Indexers {
	return nil
}

func (m *mockIndexer) AddIndexers(newIndexers gocache.Indexers) error {
	return nil
}

// Test getIndexedPolicies
func TestGetIndexedPolicies(t *testing.T) {
	policy1 := &v1.ModelAccessPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "policy1"},
	}
	policy2 := &v1.ModelAccessPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "policy2"},
	}

	tests := []struct {
		name          string
		index         string
		key           string
		indexerData   map[string]map[string][]interface{}
		expectedCount int
	}{
		{
			name:  "policies found",
			index: mapUserIndex,
			key:   "user1",
			indexerData: map[string]map[string][]interface{}{
				mapUserIndex: {
					"user1": {policy1, policy2},
				},
			},
			expectedCount: 2,
		},
		{
			name:  "no policies found",
			index: mapUserIndex,
			key:   "user2",
			indexerData: map[string]map[string][]interface{}{
				mapUserIndex: {
					"user1": {policy1},
				},
			},
			expectedCount: 0,
		},
		{
			name:          "empty indexer",
			index:         mapUserIndex,
			key:           "user1",
			indexerData:   map[string]map[string][]interface{}{},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockIdx := newMockIndexer()
			mockIdx.data = tt.indexerData

			h := &Helper{
				mapIndexer: mockIdx,
			}

			policies, err := h.getIndexedPolicies(tt.index, tt.key)
			require.NoError(t, err)
			assert.Len(t, policies, tt.expectedCount)
		})
	}
}

// Test getAliasModels
func TestGetAliasModels(t *testing.T) {
	tests := []struct {
		name     string
		values   []string
		expected map[string]string
	}{
		{
			name: "valid aliases",
			values: []string{
				"llm/m1-gpt-4o",
				"vision/m1-vision-model",
			},
			expected: map[string]string{
				"llm":    "m1-gpt-4o",
				"vision": "m1-vision-model",
			},
		},
		{
			name:     "empty values",
			values:   []string{},
			expected: map[string]string{},
		},
		{
			name: "invalid format (no slash)",
			values: []string{
				"invalid",
			},
			expected: map[string]string{},
		},
		{
			name: "invalid model ID",
			values: []string{
				"llm/invalid-model",
			},
			expected: map[string]string{},
		},
		{
			name: "unknown alias type",
			values: []string{
				"unknown/m1-gpt-4o",
			},
			expected: map[string]string{},
		},
		{
			name: "mixed valid and invalid",
			values: []string{
				"llm/m1-gpt-4o",
				"invalid",
				"vision/m1-vision-model",
			},
			expected: map[string]string{
				"llm":    "m1-gpt-4o",
				"vision": "m1-vision-model",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockIdx := newMockIndexer()
			mockIdx.values = tt.values

			h := &Helper{
				dmaIndexer: mockIdx,
			}

			result := h.getAliasModels()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test getUserPolicies
func TestGetUserPolicies(t *testing.T) {
	policy1 := &v1.ModelAccessPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "policy1"},
	}

	mockIdx := newMockIndexer()
	mockIdx.data = map[string]map[string][]interface{}{
		mapUserIndex: {
			"user1": {policy1},
		},
	}

	h := &Helper{
		mapIndexer: mockIdx,
	}

	policies, err := h.getUserPolicies("user1")
	require.NoError(t, err)
	assert.Len(t, policies, 1)
	assert.Equal(t, "policy1", policies[0].Name)
}

// Test getGroupPolicies
func TestGetGroupPolicies(t *testing.T) {
	policy1 := &v1.ModelAccessPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "policy1"},
	}

	mockIdx := newMockIndexer()
	mockIdx.data = map[string]map[string][]interface{}{
		mapGroupIndex: {
			"group1": {policy1},
		},
	}

	h := &Helper{
		mapIndexer: mockIdx,
	}

	policies, err := h.getGroupPolicies("group1")
	require.NoError(t, err)
	assert.Len(t, policies, 1)
	assert.Equal(t, "policy1", policies[0].Name)
}

// Test getWildcardUserPolicies
func TestGetWildcardUserPolicies(t *testing.T) {
	policy1 := &v1.ModelAccessPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "policy1"},
	}

	mockIdx := newMockIndexer()
	mockIdx.data = map[string]map[string][]interface{}{
		mapSelectorIndex: {
			"*": {policy1},
		},
	}

	h := &Helper{
		mapIndexer: mockIdx,
	}

	policies, err := h.getWildcardUserPolicies()
	require.NoError(t, err)
	assert.Len(t, policies, 1)
	assert.Equal(t, "policy1", policies[0].Name)
}

// Test GetUserAllowedModels with various scenarios
func TestGetUserAllowedModels(t *testing.T) {
	tests := []struct {
		name                string
		user                kuser.Info
		mapIndexerData      map[string]map[string][]interface{}
		dmaIndexerValues    []string
		expectedModels      map[string]bool
		expectedAllowAll    bool
		expectNonEmptySet   bool
		expectAtLeastModels []string
	}{
		{
			name: "wildcard policy with wildcard model",
			user: &kuser.DefaultInfo{
				UID: "user1",
			},
			mapIndexerData: map[string]map[string][]interface{}{
				mapSelectorIndex: {
					"*": {
						&v1.ModelAccessPolicy{
							Spec: v1.ModelAccessPolicySpec{
								Manifest: types.ModelAccessPolicyManifest{
									Subjects: []types.Subject{
										{Type: types.SubjectTypeSelector, ID: "*"},
									},
									Models: []types.ModelResource{
										{ID: "*"},
									},
								},
							},
						},
					},
				},
			},
			expectedAllowAll: true,
		},
		{
			name: "user policy with specific models",
			user: &kuser.DefaultInfo{
				UID: "user1",
			},
			mapIndexerData: map[string]map[string][]interface{}{
				mapUserIndex: {
					"user1": {
						&v1.ModelAccessPolicy{
							Spec: v1.ModelAccessPolicySpec{
								Manifest: types.ModelAccessPolicyManifest{
									Subjects: []types.Subject{
										{Type: types.SubjectTypeUser, ID: "user1"},
									},
									Models: []types.ModelResource{
										{ID: "m1-gpt-4o"},
										{ID: "m1-claude-sonnet"},
									},
								},
							},
						},
					},
				},
			},
			expectedModels: map[string]bool{
				"m1-gpt-4o":         true,
				"m1-claude-sonnet": true,
			},
			expectNonEmptySet: true,
		},
		{
			name: "group policy with specific model",
			user: &kuser.DefaultInfo{
				UID: "user1",
				Extra: map[string][]string{
					"auth_provider_groups": {"group1"},
				},
			},
			mapIndexerData: map[string]map[string][]interface{}{
				mapGroupIndex: {
					"group1": {
						&v1.ModelAccessPolicy{
							Spec: v1.ModelAccessPolicySpec{
								Manifest: types.ModelAccessPolicyManifest{
									Subjects: []types.Subject{
										{Type: types.SubjectTypeGroup, ID: "group1"},
									},
									Models: []types.ModelResource{
										{ID: "m1-gpt-4o"},
									},
								},
							},
						},
					},
				},
			},
			expectAtLeastModels: []string{"m1-gpt-4o"},
		},
		{
			name: "alias model reference",
			user: &kuser.DefaultInfo{
				UID: "user1",
			},
			mapIndexerData: map[string]map[string][]interface{}{
				mapUserIndex: {
					"user1": {
						&v1.ModelAccessPolicy{
							Spec: v1.ModelAccessPolicySpec{
								Manifest: types.ModelAccessPolicyManifest{
									Subjects: []types.Subject{
										{Type: types.SubjectTypeUser, ID: "user1"},
									},
									Models: []types.ModelResource{
										{ID: "obot://llm"},
									},
								},
							},
						},
					},
				},
			},
			dmaIndexerValues: []string{
				"llm/m1-gpt-4o",
			},
			expectAtLeastModels: []string{"m1-gpt-4o"},
		},
		{
			name: "no policies for user",
			user: &kuser.DefaultInfo{
				UID: "user1",
			},
			mapIndexerData: map[string]map[string][]interface{}{},
			expectedModels: map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapIdx := newMockIndexer()
			mapIdx.data = tt.mapIndexerData

			dmaIdx := newMockIndexer()
			dmaIdx.values = tt.dmaIndexerValues

			h := &Helper{
				mapIndexer: mapIdx,
				dmaIndexer: dmaIdx,
			}

			models, allowAll, err := h.GetUserAllowedModels(tt.user)
			require.NoError(t, err)

			if tt.expectedAllowAll {
				assert.True(t, allowAll)
				assert.Nil(t, models)
			} else if tt.expectNonEmptySet {
				assert.False(t, allowAll)
				assert.NotNil(t, models)
				assert.Equal(t, tt.expectedModels, models)
			} else if len(tt.expectAtLeastModels) > 0 {
				assert.False(t, allowAll)
				assert.NotNil(t, models)
				for _, modelID := range tt.expectAtLeastModels {
					assert.True(t, models[modelID], "expected model %s to be in allowed models", modelID)
				}
			} else {
				assert.False(t, allowAll)
				assert.Equal(t, tt.expectedModels, models)
			}
		})
	}
}

// Test UserHasAccessToModel
func TestUserHasAccessToModel(t *testing.T) {
	tests := []struct {
		name           string
		user           kuser.Info
		modelID        string
		mapIndexerData map[string]map[string][]interface{}
		expectedAccess bool
	}{
		{
			name: "user has access via direct policy",
			user: &kuser.DefaultInfo{
				UID: "user1",
			},
			modelID: "m1-gpt-4o",
			mapIndexerData: map[string]map[string][]interface{}{
				mapUserIndex: {
					"user1": {
						&v1.ModelAccessPolicy{
							Spec: v1.ModelAccessPolicySpec{
								Manifest: types.ModelAccessPolicyManifest{
									Subjects: []types.Subject{
										{Type: types.SubjectTypeUser, ID: "user1"},
									},
									Models: []types.ModelResource{
										{ID: "m1-gpt-4o"},
									},
								},
							},
						},
					},
				},
			},
			expectedAccess: true,
		},
		{
			name: "user has access via wildcard policy",
			user: &kuser.DefaultInfo{
				UID: "user1",
			},
			modelID: "m1-gpt-4o",
			mapIndexerData: map[string]map[string][]interface{}{
				mapSelectorIndex: {
					"*": {
						&v1.ModelAccessPolicy{
							Spec: v1.ModelAccessPolicySpec{
								Manifest: types.ModelAccessPolicyManifest{
									Subjects: []types.Subject{
										{Type: types.SubjectTypeSelector, ID: "*"},
									},
									Models: []types.ModelResource{
										{ID: "*"},
									},
								},
							},
						},
					},
				},
			},
			expectedAccess: true,
		},
		{
			name: "user does not have access",
			user: &kuser.DefaultInfo{
				UID: "user1",
			},
			modelID: "m1-gpt-4o",
			mapIndexerData: map[string]map[string][]interface{}{
				mapUserIndex: {
					"user1": {
						&v1.ModelAccessPolicy{
							Spec: v1.ModelAccessPolicySpec{
								Manifest: types.ModelAccessPolicyManifest{
									Subjects: []types.Subject{
										{Type: types.SubjectTypeUser, ID: "user1"},
									},
									Models: []types.ModelResource{
										{ID: "m1-claude-sonnet"},
									},
								},
							},
						},
					},
				},
			},
			expectedAccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapIdx := newMockIndexer()
			mapIdx.data = tt.mapIndexerData

			dmaIdx := newMockIndexer()

			h := &Helper{
				mapIndexer: mapIdx,
				dmaIndexer: dmaIdx,
			}

			hasAccess, err := h.UserHasAccessToModel(tt.user, tt.modelID)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedAccess, hasAccess)
		})
	}
}
