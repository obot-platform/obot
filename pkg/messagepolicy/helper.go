package messagepolicy

import (
	"context"
	"fmt"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/backend"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

const (
	userIndex     = "user-id"
	groupIndex    = "group-id"
	selectorIndex = "selector-id"
)

type Helper struct {
	indexer    gocache.Indexer
	client     kclient.Client
	dispatcher *dispatcher.Dispatcher
	gptClient  *gptscript.GPTScript
}

func NewHelper(ctx context.Context, backend backend.Backend, client kclient.Client, dispatcher *dispatcher.Dispatcher, gptClient *gptscript.GPTScript) (*Helper, error) {
	gvk, err := backend.GroupVersionKindFor(&v1.MessagePolicy{})
	if err != nil {
		return nil, err
	}

	informer, err := backend.GetInformerForKind(ctx, gvk)
	if err != nil {
		return nil, err
	}

	if err := informer.AddIndexers(gocache.Indexers{
		userIndex:     subjectIndexFunc(types.SubjectTypeUser),
		groupIndex:    subjectIndexFunc(types.SubjectTypeGroup),
		selectorIndex: subjectIndexFunc(types.SubjectTypeSelector),
	}); err != nil {
		return nil, err
	}

	return &Helper{
		indexer:    informer.GetIndexer(),
		client:     client,
		dispatcher: dispatcher,
		gptClient:  gptClient,
	}, nil
}

// ApplicablePolicy pairs a policy's Kubernetes resource name with its manifest.
type ApplicablePolicy struct {
	ID       string // Kubernetes resource name (e.g., "mp1-abc123")
	Manifest types.MessagePolicyManifest
}

// GetApplicablePolicies returns all policies that apply to the given user and direction.
func (h *Helper) GetApplicablePolicies(user kuser.Info, direction types.PolicyDirection) ([]ApplicablePolicy, error) {
	seen := make(map[string]struct{})
	var result []ApplicablePolicy

	collect := func(policies []v1.MessagePolicy) {
		for _, p := range policies {
			if _, ok := seen[p.Name]; ok {
				continue
			}
			d := p.Spec.Manifest.Direction
			if d == direction || d == types.PolicyDirectionBoth {
				seen[p.Name] = struct{}{}
				result = append(result, ApplicablePolicy{
					ID:       p.Name,
					Manifest: p.Spec.Manifest,
				})
			}
		}
	}

	// Wildcard policies
	wildcardPolicies, err := h.getIndexedPolicies(selectorIndex, "*")
	if err != nil {
		return nil, err
	}
	collect(wildcardPolicies)

	// User-specific policies
	userPolicies, err := h.getIndexedPolicies(userIndex, user.GetUID())
	if err != nil {
		return nil, err
	}
	collect(userPolicies)

	// Group-based policies
	for groupID := range authGroupSet(user) {
		groupPolicies, err := h.getIndexedPolicies(groupIndex, groupID)
		if err != nil {
			return nil, err
		}
		collect(groupPolicies)
	}

	return result, nil
}

func (h *Helper) getIndexedPolicies(index, key string) ([]v1.MessagePolicy, error) {
	policies, err := h.indexer.ByIndex(index, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get message policies for index %s/%s: %w", index, key, err)
	}

	result := make([]v1.MessagePolicy, 0, len(policies))
	for _, policy := range policies {
		if res, ok := policy.(*v1.MessagePolicy); ok {
			result = append(result, *res)
		}
	}

	return result, nil
}

func subjectIndexFunc(subjectType types.SubjectType) gocache.IndexFunc {
	return func(obj any) ([]string, error) {
		policy := obj.(*v1.MessagePolicy)
		if !policy.DeletionTimestamp.IsZero() {
			return nil, nil
		}

		var keys []string
		for _, subject := range policy.Spec.Manifest.Subjects {
			if subject.Type == subjectType {
				keys = append(keys, subject.ID)
			}
		}

		return keys, nil
	}
}

// authGroupSet returns a set of auth provider groups for a given user.
func authGroupSet(user kuser.Info) map[string]struct{} {
	groups := user.GetExtra()["auth_provider_groups"]
	set := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		set[group] = struct{}{}
	}
	return set
}
