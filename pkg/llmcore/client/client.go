package client

import (
	"context"
	"encoding/json"
	"fmt"

	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Client struct {
	storage kclient.Client
	codec   ConfigCodec
}

// ConfigCodec protects complete configuration documents with Obot's credential encryption.
type ConfigCodec interface {
	SealGatewayConfig(context.Context, string, []byte) ([]byte, error)
	OpenGatewayConfig(context.Context, string, []byte) ([]byte, error)
}

// New requires a codec so every client follows the configured credential policy.
// The codec may allow plaintext when application encryption is disabled.
func New(storage kclient.Client, codec ConfigCodec) (*Client, error) {
	if codec == nil {
		return nil, fmt.Errorf("a gateway configuration codec is required")
	}
	return &Client{storage: storage, codec: codec}, nil
}

func (s *Client) encode(ctx context.Context, id string, v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	data, err = s.codec.SealGatewayConfig(ctx, id, data)
	return string(data), err
}

func (s *Client) decode(ctx context.Context, id, raw string, v any) error {
	data, err := s.codec.OpenGatewayConfig(ctx, id, []byte(raw))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
