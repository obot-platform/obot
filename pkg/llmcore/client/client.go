package client

import (
	"context"
	"encoding/json"

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

func New(storage kclient.Client, codecs ...ConfigCodec) *Client {
	var codec ConfigCodec
	if len(codecs) > 0 {
		codec = codecs[0]
	}
	return &Client{storage: storage, codec: codec}
}
func (s *Client) encode(ctx context.Context, id string, v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	if s.codec != nil {
		data, err = s.codec.SealGatewayConfig(ctx, id, data)
	}
	return string(data), err
}

func (s *Client) decode(ctx context.Context, id, raw string, v any) error {
	data := []byte(raw)
	if s.codec != nil {
		var err error
		data, err = s.codec.OpenGatewayConfig(ctx, id, data)
		if err != nil {
			return err
		}
	}
	return json.Unmarshal(data, v)
}

// Load assembles independently versioned resources. It does not provide a
// transactional snapshot; runtime reconciliation must tolerate temporary
// missing dependencies and retry until the desired configuration converges.
func (s *Client) Load(ctx context.Context) (Config, error) {
	cfg, err := s.loadResources(ctx)
	return cfg, err
}
