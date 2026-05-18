package imagepullsecrets

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
)

type ECRAuthorizationClient interface {
	GetAuthorizationToken(context.Context, *ecr.GetAuthorizationTokenInput, ...func(*ecr.Options)) (*ecr.GetAuthorizationTokenOutput, error)
}

type ECRRefreshResult struct {
	DockerConfigJSON  []byte
	TokenExpiresAt    *time.Time
	RegistryEndpoints []string
}

func FetchECRDockerConfig(ctx context.Context, client ECRAuthorizationClient) (*ECRRefreshResult, error) {
	// ECR returns Docker-compatible auth tokens. We never log or store the token
	// itself outside the target kubernetes.io/dockerconfigjson Secret.
	output, err := client.GetAuthorizationToken(ctx, &ecr.GetAuthorizationTokenInput{})
	if err != nil {
		return nil, err
	}
	if len(output.AuthorizationData) == 0 {
		return nil, errors.New("ecr returned no authorization data")
	}

	auths := map[string]dockerAuth{}
	var endpoints []string
	var earliestExpiry *time.Time
	for i, data := range output.AuthorizationData {
		endpoint := strings.TrimRight(strings.TrimSpace(aws.ToString(data.ProxyEndpoint)), "/")
		if endpoint == "" {
			return nil, fmt.Errorf("ECR authorization data %d is missing proxy endpoint", i)
		}
		username, password, err := decodeECRAuthorizationToken(data)
		if err != nil {
			return nil, fmt.Errorf("invalid ECR authorization data for %s: %w", endpoint, err)
		}
		auths[endpoint] = dockerAuth{
			Username: username,
			Password: password,
			Auth:     base64.StdEncoding.EncodeToString([]byte(username + ":" + password)),
		}
		endpoints = append(endpoints, endpoint)
		if data.ExpiresAt != nil {
			expiresAt := data.ExpiresAt.UTC()
			if earliestExpiry == nil || expiresAt.Before(*earliestExpiry) {
				earliestExpiry = &expiresAt
			}
		}
	}
	slices.Sort(endpoints)
	endpoints = slices.Compact(endpoints)

	dockerConfigJSON, err := buildDockerConfigJSON(auths)
	if err != nil {
		return nil, err
	}

	return &ECRRefreshResult{
		DockerConfigJSON:  dockerConfigJSON,
		TokenExpiresAt:    earliestExpiry,
		RegistryEndpoints: endpoints,
	}, nil
}

func decodeECRAuthorizationToken(data ecrtypes.AuthorizationData) (string, string, error) {
	token := strings.TrimSpace(aws.ToString(data.AuthorizationToken))
	if token == "" {
		return "", "", errors.New("authorization token is missing")
	}
	decoded, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return "", "", fmt.Errorf("authorization token is not valid base64: %w", err)
	}
	username, password, ok := strings.Cut(string(decoded), ":")
	if !ok || username == "" || password == "" {
		return "", "", errors.New("authorization token payload is invalid")
	}
	return username, password, nil
}
