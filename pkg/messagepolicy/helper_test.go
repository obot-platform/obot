package messagepolicy

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

func TestGetApplicablePoliciesDirectUserMatch(t *testing.T) {
	helper := newTestHelper(t,
		newPolicy("p1", "No PII", types.PolicyDirectionUserMessage,
			[]types.Subject{{Type: types.SubjectTypeUser, ID: "user1"}}),
	)

	policies, err := helper.GetApplicablePolicies(testUser("user1"), types.PolicyDirectionUserMessage)
	require.NoError(t, err)
	require.Len(t, policies, 1)
	assert.Equal(t, "No PII", policies[0].Definition)
}

func TestGetApplicablePoliciesWildcardSubject(t *testing.T) {
	helper := newTestHelper(t,
		newPolicy("p1", "No PII", types.PolicyDirectionUserMessage,
			[]types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}}),
	)

	policies, err := helper.GetApplicablePolicies(testUser("anyone"), types.PolicyDirectionUserMessage)
	require.NoError(t, err)
	require.Len(t, policies, 1)
}

func TestGetApplicablePoliciesGroupMatch(t *testing.T) {
	helper := newTestHelper(t,
		newPolicy("p1", "Economy only", types.PolicyDirectionBoth,
			[]types.Subject{{Type: types.SubjectTypeGroup, ID: "eng"}}),
	)

	policies, err := helper.GetApplicablePolicies(testUser("user1", "eng"), types.PolicyDirectionUserMessage)
	require.NoError(t, err)
	require.Len(t, policies, 1)
	assert.Equal(t, "Economy only", policies[0].Definition)
}

func TestGetApplicablePoliciesDirectionFiltering(t *testing.T) {
	helper := newTestHelper(t,
		newPolicy("p-input", "Input only", types.PolicyDirectionUserMessage,
			[]types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}}),
		newPolicy("p-output", "Output only", types.PolicyDirectionLLMResponse,
			[]types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}}),
		newPolicy("p-both", "Both directions", types.PolicyDirectionBoth,
			[]types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}}),
	)

	// Query for user-message direction: should get input + both
	policies, err := helper.GetApplicablePolicies(testUser("user1"), types.PolicyDirectionUserMessage)
	require.NoError(t, err)
	require.Len(t, policies, 2)

	defs := make(map[string]bool)
	for _, p := range policies {
		defs[p.Definition] = true
	}
	assert.True(t, defs["Input only"])
	assert.True(t, defs["Both directions"])
	assert.False(t, defs["Output only"])

	// Query for llm-response direction: should get output + both
	policies, err = helper.GetApplicablePolicies(testUser("user1"), types.PolicyDirectionLLMResponse)
	require.NoError(t, err)
	require.Len(t, policies, 2)

	defs = make(map[string]bool)
	for _, p := range policies {
		defs[p.Definition] = true
	}
	assert.True(t, defs["Output only"])
	assert.True(t, defs["Both directions"])
	assert.False(t, defs["Input only"])
}

func TestGetApplicablePoliciesDeduplication(t *testing.T) {
	// Policy matches via both user and group — should only appear once
	helper := newTestHelper(t,
		newPolicy("p1", "No PII", types.PolicyDirectionUserMessage,
			[]types.Subject{
				{Type: types.SubjectTypeUser, ID: "user1"},
				{Type: types.SubjectTypeGroup, ID: "eng"},
			}),
	)

	policies, err := helper.GetApplicablePolicies(testUser("user1", "eng"), types.PolicyDirectionUserMessage)
	require.NoError(t, err)
	require.Len(t, policies, 1)
}

func TestGetApplicablePoliciesNoMatch(t *testing.T) {
	helper := newTestHelper(t,
		newPolicy("p1", "No PII", types.PolicyDirectionUserMessage,
			[]types.Subject{{Type: types.SubjectTypeUser, ID: "user2"}}),
	)

	policies, err := helper.GetApplicablePolicies(testUser("user1"), types.PolicyDirectionUserMessage)
	require.NoError(t, err)
	assert.Empty(t, policies)
}

func TestGetApplicablePoliciesDeletedPolicyExcluded(t *testing.T) {
	helper := newTestHelper(t,
		newPolicy("p1", "No PII", types.PolicyDirectionUserMessage,
			[]types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
			withDeleted()),
	)

	policies, err := helper.GetApplicablePolicies(testUser("user1"), types.PolicyDirectionUserMessage)
	require.NoError(t, err)
	assert.Empty(t, policies)
}

func TestGetApplicablePoliciesMultipleGroups(t *testing.T) {
	helper := newTestHelper(t,
		newPolicy("p1", "Eng policy", types.PolicyDirectionUserMessage,
			[]types.Subject{{Type: types.SubjectTypeGroup, ID: "eng"}}),
		newPolicy("p2", "Ops policy", types.PolicyDirectionUserMessage,
			[]types.Subject{{Type: types.SubjectTypeGroup, ID: "ops"}}),
	)

	// User in both groups gets both policies
	policies, err := helper.GetApplicablePolicies(testUser("user1", "eng", "ops"), types.PolicyDirectionUserMessage)
	require.NoError(t, err)
	require.Len(t, policies, 2)

	// User in only eng gets one policy
	policies, err = helper.GetApplicablePolicies(testUser("user1", "eng"), types.PolicyDirectionUserMessage)
	require.NoError(t, err)
	require.Len(t, policies, 1)
	assert.Equal(t, "Eng policy", policies[0].Definition)
}

// --- test helpers ---

func newTestHelper(t *testing.T, policies ...*v1.MessagePolicy) *Helper {
	t.Helper()

	indexer := gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, gocache.Indexers{
		userIndex:     subjectIndexFunc(types.SubjectTypeUser),
		groupIndex:    subjectIndexFunc(types.SubjectTypeGroup),
		selectorIndex: subjectIndexFunc(types.SubjectTypeSelector),
	})

	for _, p := range policies {
		require.NoError(t, indexer.Add(p))
	}

	return &Helper{indexer: indexer}
}

type policyOpt func(*v1.MessagePolicy)

func withDeleted() policyOpt {
	return func(p *v1.MessagePolicy) {
		p.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	}
}

func newPolicy(name, definition string, direction types.PolicyDirection, subjects []types.Subject, opts ...policyOpt) *v1.MessagePolicy {
	p := &v1.MessagePolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: v1.MessagePolicySpec{
			Manifest: types.MessagePolicyManifest{
				DisplayName: name,
				Definition:  definition,
				Direction:   direction,
				Subjects:    subjects,
			},
		},
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func testUser(userID string, groups ...string) kuser.Info {
	return &kuser.DefaultInfo{
		UID: userID,
		Extra: map[string][]string{
			"auth_provider_groups": groups,
		},
	}
}
