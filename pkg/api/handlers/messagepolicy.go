package handlers

import (
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MessagePolicyHandler struct{}

func NewMessagePolicyHandler() *MessagePolicyHandler {
	return &MessagePolicyHandler{}
}

// List returns all message policies.
func (*MessagePolicyHandler) List(req api.Context) error {
	var list v1.MessagePolicyList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list message policies: %w", err)
	}

	items := make([]types.MessagePolicy, 0, len(list.Items))
	for _, item := range list.Items {
		items = append(items, convertMessagePolicy(item))
	}

	return req.Write(types.MessagePolicyList{
		Items: items,
	})
}

// Get returns a specific message policy by ID.
func (*MessagePolicyHandler) Get(req api.Context) error {
	policyID := req.PathValue("id")

	var policy v1.MessagePolicy
	if err := req.Get(&policy, policyID); err != nil {
		return fmt.Errorf("failed to get message policy: %w", err)
	}

	return req.Write(convertMessagePolicy(policy))
}

// Create creates a new message policy.
func (*MessagePolicyHandler) Create(req api.Context) error {
	var manifest types.MessagePolicyManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read message policy manifest: %v", err)
	}

	if err := manifest.Validate(); err != nil {
		return types.NewErrBadRequest("invalid message policy manifest: %v", err)
	}

	policy := v1.MessagePolicy{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.MessagePolicyPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.MessagePolicySpec{
			Manifest: manifest,
		},
	}

	if err := req.Create(&policy); err != nil {
		return fmt.Errorf("failed to create message policy: %w", err)
	}

	return req.Write(convertMessagePolicy(policy))
}

// Update updates an existing message policy.
func (*MessagePolicyHandler) Update(req api.Context) error {
	policyID := req.PathValue("id")

	var manifest types.MessagePolicyManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read message policy manifest: %v", err)
	}

	if err := manifest.Validate(); err != nil {
		return types.NewErrBadRequest("invalid message policy manifest: %v", err)
	}

	var existing v1.MessagePolicy
	if err := req.Get(&existing, policyID); err != nil {
		return types.NewErrBadRequest("failed to get message policy: %v", err)
	}

	existing.Spec.Manifest = manifest
	if err := req.Update(&existing); err != nil {
		return fmt.Errorf("failed to update message policy: %w", err)
	}

	return req.Write(convertMessagePolicy(existing))
}

// Delete deletes a message policy.
func (*MessagePolicyHandler) Delete(req api.Context) error {
	policyID := req.PathValue("id")

	return req.Delete(&v1.MessagePolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyID,
			Namespace: req.Namespace(),
		},
	})
}

func convertMessagePolicy(policy v1.MessagePolicy) types.MessagePolicy {
	return types.MessagePolicy{
		Metadata:              MetadataFrom(&policy),
		MessagePolicyManifest: policy.Spec.Manifest,
	}
}
