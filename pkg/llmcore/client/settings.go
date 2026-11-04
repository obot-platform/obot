package client

import (
	"bytes"
	"context"
	"encoding/json"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// settingsCodecID binds the global settings payload to its credential envelope.
const (
	settingsCodecID = "settings"
)

func (s *Client) GetSettings(ctx context.Context) (map[string]json.RawMessage, error) {
	var obj v1.LLMGatewayConfig
	settings := map[string]json.RawMessage{}
	err := s.storage.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.LLMGatewayConfigName}, &obj)
	if apierrors.IsNotFound(err) {
		return settings, nil
	}
	if err != nil {
		return nil, err
	}

	if err := s.decode(ctx, settingsCodecID, obj.Spec.Payload, &settings); err != nil {
		return nil, err
	}
	if settings == nil {
		settings = map[string]json.RawMessage{}
	}
	return settings, nil
}

// MutateSettings attempts one edit of the global settings singleton. Conflicts
// and concurrent creation (AlreadyExists) are returned for the caller to handle.
// The callback must not have externally visible side effects.
func (s *Client) MutateSettings(ctx context.Context, change func(map[string]json.RawMessage) error) error {
	var obj v1.LLMGatewayConfig
	err := s.storage.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.LLMGatewayConfigName}, &obj)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	exists := err == nil
	settings := map[string]json.RawMessage{}
	if exists {
		if err := s.decode(ctx, settingsCodecID, obj.Spec.Payload, &settings); err != nil {
			return err
		}
	}

	if settings == nil {
		settings = map[string]json.RawMessage{}
	}

	before, err := json.Marshal(settings)
	if err != nil {
		return err
	}

	if err := change(settings); err != nil {
		return err
	}

	after, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	if bytes.Equal(before, after) {
		return nil
	}

	payload, err := s.encode(ctx, settingsCodecID, settings)
	if err != nil {
		return err
	}

	obj.Spec = v1.LLMGatewayResourceSpec{ID: system.LLMGatewayConfigName, Payload: payload}
	if !exists {
		obj.Name = system.LLMGatewayConfigName
		obj.Namespace = system.DefaultNamespace
		return s.storage.Create(ctx, &obj)
	}
	return s.storage.Update(ctx, &obj)
}
