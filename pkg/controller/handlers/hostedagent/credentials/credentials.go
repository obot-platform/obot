// Package credentials issues the credential a hosted agent sandbox uses to
// authenticate to Obot.
//
// The credential authenticates as the agent rather than as its owner, so a
// compromised sandbox reaches only what that one instance was configured with
// instead of everything its owner can reach.
package credentials

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
)

const (
	credentialContext = "hosted-agent-credential"
	keyValue          = "OBOT_API_KEY"
	keyID             = "OBOT_API_KEY_ID"
)

type Issuer struct {
	gatewayClient *client.Client
}

func New(gatewayClient *client.Client) *Issuer {
	return &Issuer{gatewayClient: gatewayClient}
}

// EnsureInstanceCredential returns the instance's credential, minting one only
// when none exists.
//
// The plaintext is kept in Obot's encrypted credential store, mirroring what
// nanobot agents do, because an API key's secret is not recoverable from its
// database row. Every reconcile therefore returns the same value and the same
// version, which is what keeps the desired revision stable; reissuing would
// change the version and restart the sandbox.
func (i *Issuer) EnsureInstanceCredential(ctx context.Context, instanceID, ownerUserID string) (string, string, error) {
	if i == nil || i.gatewayClient == nil {
		return "", "", nil
	}
	if instanceID == "" {
		return "", "", fmt.Errorf("instance ID is required")
	}

	stored, err := i.gatewayClient.RevealCredential(ctx, []string{credentialContext}, instanceID)
	if err == nil && stored.Secrets[keyValue] != "" {
		if valid, err := i.stillValid(ctx, stored.Secrets[keyValue]); err != nil {
			return "", "", err
		} else if valid {
			return stored.Secrets[keyValue], stored.Secrets[keyID], nil
		}
		// The key was revoked out of band. Fall through and mint a new one; the
		// changed version restarts the sandbox, which is the only way it will
		// pick the new credential up.
	} else if err != nil {
		if _, ok := errors.AsType[client.CredentialNotFoundError](err); !ok {
			return "", "", fmt.Errorf("reveal hosted agent credential: %w", err)
		}
	}

	// The owner is recorded for attribution and cleanup. It confers none of
	// that user's access: authentication resolves the instance, not the user.
	var owner uint64
	if ownerUserID != "" {
		owner, _ = strconv.ParseUint(ownerUserID, 10, 64)
	}

	created, err := i.gatewayClient.CreateHostedAgentAPIKey(ctx, instanceID, uint(owner), "hosted-agent-"+instanceID)
	if err != nil {
		return "", "", fmt.Errorf("create hosted agent credential: %w", err)
	}

	version := strconv.FormatUint(uint64(created.ID), 10)
	if err := i.gatewayClient.UpsertCredential(ctx, gatewaytypes.Credential{
		Context: credentialContext,
		Name:    instanceID,
		Secrets: map[string]string{
			keyValue: created.Key,
			keyID:    version,
		},
	}); err != nil {
		return "", "", fmt.Errorf("store hosted agent credential: %w", err)
	}

	return created.Key, version, nil
}

// RevokeInstanceCredential revokes the key and deletes its stored plaintext.
// Revocation immediately invalidates a key, so a leaked credential stops working at once
// rather than waiting for an expiry these keys deliberately do not have.
func (i *Issuer) RevokeInstanceCredential(ctx context.Context, instanceID string) error {
	if i == nil || i.gatewayClient == nil {
		return nil
	}

	var errs []error
	if err := i.gatewayClient.RevokeHostedAgentAPIKeys(ctx, instanceID); err != nil {
		errs = append(errs, err)
	}
	// DeleteCredential is idempotent, so a credential that is already gone is
	// not an error to filter out.
	if _, err := i.gatewayClient.DeleteCredential(ctx, credentialContext, instanceID); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (i *Issuer) stillValid(ctx context.Context, value string) (bool, error) {
	if _, err := i.gatewayClient.ValidateAPIKey(ctx, value); err != nil {
		return false, nil
	}
	return true, nil
}
