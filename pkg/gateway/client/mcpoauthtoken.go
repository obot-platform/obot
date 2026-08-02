package client

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage/value"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var mcpOAuthTokenGroupResource = schema.GroupResource{
	Group:    "obot.obot.ai",
	Resource: "mcpoauthtokens",
}

var mcpOAuthPendingStateGroupResource = schema.GroupResource{
	Group:    "obot.obot.ai",
	Resource: "mcpoauthpendingstates",
}

var (
	ErrMCPStaticOAuthTestInvalid        = errors.New("invalid static OAuth test proof")
	ErrMCPStaticOAuthCredentialExists   = errors.New("static OAuth credential already exists")
	ErrMCPStaticOAuthCredentialNotFound = errors.New("static OAuth credential does not exist")
	ErrMCPStaticOAuthEncryptionRequired = errors.New("static OAuth credential encryption is required")
	ErrMCPOAuthCatalogCredentialChanged = errors.New("catalog OAuth credential changed")
)

const mcpStaticOAuthTestStatusClaimed apitypes.MCPStaticOAuthTestStatus = "claimed"

// MCPStaticOAuthCredentialReady reports whether a stored application is
// complete and bound to the current catalog provider generation.
func MCPStaticOAuthCredentialReady(secrets map[string]string, fixedURL string) bool {
	return secrets["MCP_URL"] == fixedURL &&
		strings.TrimSpace(secrets["GENERATION"]) != "" &&
		strings.TrimSpace(secrets["CLIENT_ID"]) != "" &&
		strings.TrimSpace(secrets["CLIENT_SECRET"]) != ""
}

type MCPStaticOAuthTestStart struct {
	CallbackState string
	TestState     string
}

func (c *Client) GetMCPOAuthToken(ctx context.Context, userID, mcpID, url string) (*types.MCPOAuthToken, error) {
	var tokens []types.MCPOAuthToken
	err := c.db.WithContext(ctx).Where("mcp_id = ? AND user_id = ?", mcpID, userID).Find(&tokens).Error
	if err != nil {
		return nil, err
	}

	var token types.MCPOAuthToken
	for _, t := range tokens {
		if t.URL == url {
			token = t
			break
		}
	}

	if token.MCPID == "" {
		// We didn't find a token. If there is only one, then use that one.
		if len(tokens) != 1 {
			return nil, gorm.ErrRecordNotFound
		}

		token = tokens[0]
	}

	if err = c.decryptMCPOAuthToken(ctx, &token); err != nil {
		return nil, fmt.Errorf("failed to decrypt token: %w", err)
	}

	return &token, nil
}

func (c *Client) ReplaceMCPOAuthToken(ctx context.Context, userID, mcpID, url, oauthAuthRequestID string, oauthConf *oauth2.Config, token *oauth2.Token) error {
	return c.replaceMCPOAuthToken(ctx, userID, mcpID, url, oauthAuthRequestID, "", "", oauthConf, token)
}

// ReplaceMCPOAuthTokenWithCatalogCredentialGenerationFence rejects writes
// from an OAuth flow or refresh that began before a same-value credential replacement.
func (c *Client) ReplaceMCPOAuthTokenWithCatalogCredentialGenerationFence(ctx context.Context, userID, mcpID, url, oauthAuthRequestID, catalogEntryName, catalogCredentialGeneration string, oauthConf *oauth2.Config, token *oauth2.Token) error {
	if catalogEntryName == "" {
		return c.ReplaceMCPOAuthToken(ctx, userID, mcpID, url, oauthAuthRequestID, oauthConf, token)
	}

	releaseCatalogMutationLock, err := c.AcquireCredentialLock(ctx, system.MCPStaticOAuthCatalogMutationLock)
	if err != nil {
		return fmt.Errorf("failed to coordinate catalog OAuth token write with catalog mutation: %w", err)
	}
	defer releaseCatalogMutationLock()
	credentialKey := system.MCPOAuthCredentialName(catalogEntryName)
	release, err := c.AcquireCredentialLock(ctx, credentialKey)
	if err != nil {
		return fmt.Errorf("failed to coordinate catalog OAuth token write: %w", err)
	}
	defer release()

	if err := c.validateCatalogOAuthCredential(ctx, mcpID, url, catalogEntryName, catalogCredentialGeneration, oauthConf); err != nil {
		return ErrMCPOAuthCatalogCredentialChanged
	}

	return c.replaceMCPOAuthToken(ctx, userID, mcpID, url, oauthAuthRequestID, catalogEntryName, catalogCredentialGeneration, oauthConf, token)
}

// ValidateCatalogOAuthToken rejects a persisted grant when its catalog entry,
// provider URL, or shared application no longer matches the active catalog state.
func (c *Client) ValidateCatalogOAuthToken(ctx context.Context, mcpID, url, catalogEntryName, catalogCredentialGeneration string, oauthConf *oauth2.Config) error {
	releaseCatalogMutationLock, err := c.AcquireCredentialLock(ctx, system.MCPStaticOAuthCatalogMutationLock)
	if err != nil {
		return fmt.Errorf("failed to coordinate catalog OAuth token read with catalog mutation: %w", err)
	}
	defer releaseCatalogMutationLock()
	credentialKey := system.MCPOAuthCredentialName(catalogEntryName)
	release, err := c.AcquireCredentialLock(ctx, credentialKey)
	if err != nil {
		return fmt.Errorf("failed to coordinate catalog OAuth token read: %w", err)
	}
	defer release()
	return c.validateCatalogOAuthCredential(ctx, mcpID, url, catalogEntryName, catalogCredentialGeneration, oauthConf)
}

func (c *Client) validateCatalogOAuthCredential(ctx context.Context, mcpID, url, catalogEntryName, catalogCredentialGeneration string, oauthConf *oauth2.Config) error {
	currentEntryName, err := c.mcpCatalogEntryName(ctx, mcpID)
	if err != nil || !secureStringEqual(currentEntryName, catalogEntryName) {
		return ErrMCPOAuthCatalogCredentialChanged
	}
	currentURL, err := c.mcpCatalogEntryURL(ctx, catalogEntryName)
	if err != nil || !secureStringEqual(currentURL, url) {
		return ErrMCPOAuthCatalogCredentialChanged
	}
	credential, err := c.RevealCredential(ctx, []string{system.MCPOAuthCredentialName(catalogEntryName)}, "oauth")
	if err != nil ||
		!secureStringEqual(credential.Secrets["CLIENT_ID"], oauthConf.ClientID) ||
		!secureStringEqual(credential.Secrets["CLIENT_SECRET"], oauthConf.ClientSecret) {
		return ErrMCPOAuthCatalogCredentialChanged
	}
	credentialGeneration := credential.Secrets["GENERATION"]
	if credentialGeneration == "" || catalogCredentialGeneration == "" ||
		!secureStringEqual(credentialGeneration, catalogCredentialGeneration) {
		return ErrMCPOAuthCatalogCredentialChanged
	}
	if !secureStringEqual(credential.Secrets["MCP_URL"], url) {
		return ErrMCPOAuthCatalogCredentialChanged
	}
	return nil
}

// CatalogEntryForStaticOAuthMCP recovers the fence identity for OAuth state
// rows created before catalog entry identity was persisted.
func (c *Client) CatalogEntryForStaticOAuthMCP(ctx context.Context, mcpID, url string) (string, error) {
	entryName, err := c.mcpCatalogEntryName(ctx, mcpID)
	if err != nil {
		return "", ErrMCPOAuthCatalogCredentialChanged
	}
	if entryName == "" {
		return "", nil
	}
	entry, err := c.mcpCatalogEntry(ctx, entryName)
	if err != nil || entry.Spec.Manifest.RemoteConfig == nil {
		return "", ErrMCPOAuthCatalogCredentialChanged
	}
	if !entry.Spec.Manifest.RemoteConfig.StaticOAuthRequired {
		return "", nil
	}
	if !secureStringEqual(entry.Spec.Manifest.RemoteConfig.FixedURL, url) {
		return "", ErrMCPOAuthCatalogCredentialChanged
	}
	return entryName, nil
}

// CommitMCPOAuthPendingStateToken persists the exchanged token using the
// catalog credential identity captured by the pending state. The explicit
// OAuth auth request ID lets callers choose the completion notification that
// belongs to their flow rather than trusting a legacy state value.
func (c *Client) CommitMCPOAuthPendingStateToken(ctx context.Context, pendingState *types.MCPOAuthPendingState, oauthAuthRequestID string, oauthConf *oauth2.Config, token *oauth2.Token) error {
	catalogEntryName := pendingState.CatalogEntryName
	if catalogEntryName == "" {
		var err error
		catalogEntryName, err = c.CatalogEntryForStaticOAuthMCP(ctx, pendingState.MCPID, pendingState.URL)
		if err != nil {
			c.deleteChangedMCPOAuthPendingState(ctx, pendingState.HashedState, err)
			return err
		}
	}

	if err := c.ReplaceMCPOAuthTokenWithCatalogCredentialGenerationFence(ctx, pendingState.UserID, pendingState.MCPID, pendingState.URL, oauthAuthRequestID, catalogEntryName, pendingState.CatalogCredentialGeneration, oauthConf, token); err != nil {
		c.deleteChangedMCPOAuthPendingState(ctx, pendingState.HashedState, err)
		return err
	}

	_ = c.DeleteMCPOAuthPendingState(ctx, pendingState.HashedState)
	return nil
}

func (c *Client) deleteChangedMCPOAuthPendingState(ctx context.Context, hashedState string, err error) {
	if errors.Is(err, ErrMCPOAuthCatalogCredentialChanged) {
		_ = c.DeleteMCPOAuthPendingState(ctx, hashedState)
	}
}

// CatalogEntryForCurrentOAuthCredential identifies a legacy grant that uses
// the static app which is active for its MCP. Static-required entries fail
// closed; optional entries without an app preserve dynamic registration.
func (c *Client) CatalogEntryForCurrentOAuthCredential(ctx context.Context, userID, mcpID, url string, oauthConf *oauth2.Config) (string, error) {
	entryName, err := c.mcpCatalogEntryName(ctx, mcpID)
	if err != nil {
		return "", err
	}
	if entryName == "" {
		return "", nil
	}
	releaseCatalogMutationLock, err := c.AcquireCredentialLock(ctx, system.MCPStaticOAuthCatalogMutationLock)
	if err != nil {
		return "", fmt.Errorf("failed to coordinate legacy catalog OAuth token read with catalog mutation: %w", err)
	}
	defer releaseCatalogMutationLock()
	credentialKey := system.MCPOAuthCredentialName(entryName)
	release, err := c.AcquireCredentialLock(ctx, credentialKey)
	if err != nil {
		return "", fmt.Errorf("failed to coordinate catalog OAuth token read: %w", err)
	}
	defer release()
	entry, err := c.mcpCatalogEntry(ctx, entryName)
	if err != nil || entry.Spec.Manifest.RemoteConfig == nil || !secureStringEqual(entry.Spec.Manifest.RemoteConfig.FixedURL, url) {
		return "", ErrMCPOAuthCatalogCredentialChanged
	}
	if entry.Spec.Manifest.RemoteConfig.StaticOAuthRequired {
		return "", ErrMCPOAuthCatalogCredentialChanged
	}
	if _, err := c.GetMCPOAuthToken(ctx, userID, mcpID, url); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrMCPOAuthCatalogCredentialChanged
		}
		return "", err
	}
	credential, err := c.RevealCredential(ctx, []string{credentialKey}, "oauth")
	if err != nil {
		var notFound CredentialNotFoundError
		if errors.As(err, &notFound) {
			if entry.Spec.Manifest.RemoteConfig.StaticOAuthRequired {
				return "", ErrMCPOAuthCatalogCredentialChanged
			}
			return "", nil
		}
		return "", err
	}
	if !secureStringEqual(credential.Secrets["CLIENT_ID"], oauthConf.ClientID) ||
		!secureStringEqual(credential.Secrets["CLIENT_SECRET"], oauthConf.ClientSecret) {
		return "", ErrMCPOAuthCatalogCredentialChanged
	}
	return entryName, nil
}

func (c *Client) replaceMCPOAuthToken(ctx context.Context, userID, mcpID, url, oauthAuthRequestID, catalogEntryName, catalogCredentialGeneration string, oauthConf *oauth2.Config, token *oauth2.Token) error {
	t := &types.MCPOAuthToken{
		UserID:                      userID,
		MCPID:                       mcpID,
		URL:                         url,
		CatalogEntryName:            catalogEntryName,
		CatalogCredentialGeneration: catalogCredentialGeneration,
		OAuthAuthRequestID:          oauthAuthRequestID,
		AccessToken:                 token.AccessToken,
		TokenType:                   token.TokenType,
		RefreshToken:                token.RefreshToken,
		Expiry:                      token.Expiry,
		ExpiresIn:                   token.ExpiresIn,
		ClientID:                    oauthConf.ClientID,
		ClientSecret:                oauthConf.ClientSecret,
		Endpoint:                    oauthConf.Endpoint,
		RedirectURL:                 oauthConf.RedirectURL,
		Scopes:                      strings.Join(oauthConf.Scopes, " "),
	}

	if err := c.encryptMCPOAuthToken(ctx, t); err != nil {
		return fmt.Errorf("failed to encrypt token: %w", err)
	}

	if err := c.db.WithContext(ctx).Save(t).Error; err != nil {
		return err
	}
	return c.triggerMCPOAuthTokenChange(ctx, mcpID)
}

func (c *Client) mcpCatalogEntryName(ctx context.Context, mcpID string) (string, error) {
	if c.storageClient == nil {
		return "", fmt.Errorf("storage client is unavailable")
	}

	var server v1.MCPServer
	if err := c.storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: mcpID}, &server); err == nil {
		return server.Spec.MCPServerCatalogEntryName, nil
	} else if !apierrors.IsNotFound(err) {
		return "", err
	}

	var instance v1.MCPServerInstance
	if err := c.storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: mcpID}, &instance); err == nil {
		return instance.Spec.MCPServerCatalogEntryName, nil
	} else if !apierrors.IsNotFound(err) {
		return "", err
	}

	return "", apierrors.NewNotFound(v1.SchemeGroupVersion.WithResource("mcpservers").GroupResource(), mcpID)
}

func (c *Client) mcpCatalogEntryURL(ctx context.Context, entryName string) (string, error) {
	entry, err := c.mcpCatalogEntry(ctx, entryName)
	if err != nil {
		return "", err
	}
	if entry.Spec.Manifest.RemoteConfig == nil {
		return "", fmt.Errorf("catalog entry has no remote configuration")
	}
	return entry.Spec.Manifest.RemoteConfig.FixedURL, nil
}

func (c *Client) mcpCatalogEntry(ctx context.Context, entryName string) (*v1.MCPServerCatalogEntry, error) {
	var entry v1.MCPServerCatalogEntry
	if err := c.storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: entryName}, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

func (c *Client) DeleteMCPOAuthTokenForURL(ctx context.Context, userID, mcpID, mcpURL string) error {
	if err := c.db.WithContext(ctx).Delete(&types.MCPOAuthToken{}, "user_id = ? AND mcp_id = ? AND (url = ? OR url = ?)", userID, mcpID, mcpURL, "").Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return c.triggerMCPOAuthTokenChange(ctx, mcpID)
}

func (c *Client) DeleteMCPOAuthTokens(ctx context.Context, userID, mcpID string) error {
	if err := c.db.WithContext(ctx).Delete(&types.MCPOAuthToken{}, "user_id = ? AND mcp_id = ?", userID, mcpID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return c.triggerMCPOAuthTokenChange(ctx, mcpID)
}

func (c *Client) DeleteMCPOAuthTokenForAllUsers(ctx context.Context, mcpID string) error {
	if err := c.db.WithContext(ctx).Delete(&types.MCPOAuthToken{}, "mcp_id = ?", mcpID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return c.triggerMCPOAuthTokenChange(ctx, mcpID)
}

func (c *Client) triggerMCPOAuthTokenChange(ctx context.Context, mcpID string) error {
	if mcpID == "" || c.mcpOAuthTokenTrigger == nil {
		return nil
	}
	return c.mcpOAuthTokenTrigger(ctx, mcpID)
}

// Pending state methods

func (c *Client) CreateMCPStaticOAuthTest(ctx context.Context, userID, mcpID, mcpURL, verifier string, oauthConf *oauth2.Config) (MCPStaticOAuthTestStart, error) {
	start := MCPStaticOAuthTestStart{
		CallbackState: strings.ToLower(rand.Text()),
		TestState:     strings.ToLower(rand.Text()),
	}
	ps := &types.MCPOAuthPendingState{
		HashedState:              hashMCPStaticOAuthValue(start.CallbackState),
		State:                    start.CallbackState,
		StaticOAuthTestStateHash: hashMCPStaticOAuthValue(start.TestState),
		Verifier:                 verifier,
		UserID:                   userID,
		MCPID:                    mcpID,
		URL:                      mcpURL,
		ClientID:                 oauthConf.ClientID,
		ClientSecret:             oauthConf.ClientSecret,
		AuthURL:                  oauthConf.Endpoint.AuthURL,
		TokenURL:                 oauthConf.Endpoint.TokenURL,
		AuthStyle:                oauthConf.Endpoint.AuthStyle,
		RedirectURL:              oauthConf.RedirectURL,
		Scopes:                   strings.Join(oauthConf.Scopes, " "),
		StaticOAuthTest:          true,
		StaticOAuthTestStatus:    apitypes.MCPStaticOAuthTestStatusPending,
	}

	if err := c.encryptMCPOAuthPendingState(ctx, ps); err != nil {
		return MCPStaticOAuthTestStart{}, fmt.Errorf("failed to encrypt static OAuth test: %w", err)
	}
	if err := c.db.WithContext(ctx).Create(ps).Error; err != nil {
		return MCPStaticOAuthTestStart{}, err
	}
	return start, nil
}

func (c *Client) GetMCPStaticOAuthTestStatus(ctx context.Context, testState, userID, mcpID string) (apitypes.MCPStaticOAuthTestResult, error) {
	var ps types.MCPOAuthPendingState
	if err := c.db.WithContext(ctx).
		Where("static_o_auth_test_state_hash = ?", hashMCPStaticOAuthValue(testState)).
		First(&ps).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apitypes.MCPStaticOAuthTestResult{}, ErrMCPStaticOAuthTestInvalid
		}
		return apitypes.MCPStaticOAuthTestResult{}, err
	}
	if err := c.decryptMCPOAuthPendingState(ctx, &ps); err != nil {
		return apitypes.MCPStaticOAuthTestResult{}, fmt.Errorf("failed to decrypt static OAuth test: %w", err)
	}
	if !ps.StaticOAuthTest || !secureStringEqual(ps.UserID, userID) || !secureStringEqual(ps.MCPID, mcpID) {
		return apitypes.MCPStaticOAuthTestResult{}, ErrMCPStaticOAuthTestInvalid
	}
	expiresAt := ps.CreatedAt.Add(pendingStateTTL)
	if time.Since(ps.CreatedAt) >= pendingStateTTL {
		return apitypes.MCPStaticOAuthTestResult{
			Status:          apitypes.MCPStaticOAuthTestStatusFailed,
			FailureCategory: apitypes.MCPStaticOAuthTestFailureExpired,
			ExpiresAt:       apitypes.Time{Time: expiresAt},
		}, nil
	}
	status := ps.StaticOAuthTestStatus
	if status == mcpStaticOAuthTestStatusClaimed {
		status = apitypes.MCPStaticOAuthTestStatusPending
	}
	return apitypes.MCPStaticOAuthTestResult{
		Status:          status,
		FailureCategory: ps.StaticOAuthTestFailureCategory,
		Proof:           ps.StaticOAuthSaveProof,
		ExpiresAt:       apitypes.Time{Time: expiresAt},
	}, nil
}

// ClaimMCPStaticOAuthTest atomically admits one unexpired callback for provider exchange.
func (c *Client) ClaimMCPStaticOAuthTest(ctx context.Context, state string) (*types.MCPOAuthPendingState, error) {
	hashedState := fmt.Sprintf("%x", sha256.Sum256([]byte(state)))
	result := c.db.WithContext(ctx).
		Model(&types.MCPOAuthPendingState{}).
		Where("hashed_state = ? AND static_o_auth_test = ? AND static_o_auth_test_status = ? AND created_at >= ?", hashedState, true, apitypes.MCPStaticOAuthTestStatusPending, time.Now().Add(-pendingStateTTL)).
		Update("static_o_auth_test_status", mcpStaticOAuthTestStatusClaimed)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, ErrMCPStaticOAuthTestInvalid
	}

	return c.GetMCPOAuthPendingState(ctx, state)
}

func (c *Client) CompleteMCPStaticOAuthTest(ctx context.Context, state string, status apitypes.MCPStaticOAuthTestStatus, failureCategory apitypes.MCPStaticOAuthTestFailureCategory) error {
	if !validMCPStaticOAuthTestCompletion(status, failureCategory) {
		return ErrMCPStaticOAuthTestInvalid
	}

	hashedState := hashMCPStaticOAuthValue(state)
	completedAt := time.Now()
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var pending types.MCPOAuthPendingState
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("hashed_state = ? AND static_o_auth_test = ? AND static_o_auth_test_status IN ? AND created_at >= ?", hashedState, true, []apitypes.MCPStaticOAuthTestStatus{apitypes.MCPStaticOAuthTestStatusPending, mcpStaticOAuthTestStatusClaimed}, completedAt.Add(-pendingStateTTL)).
			First(&pending).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrMCPStaticOAuthTestInvalid
			}
			return err
		}

		updates := map[string]any{
			"static_o_auth_test_status":           status,
			"static_o_auth_test_failure_category": failureCategory,
			"static_o_auth_test_completed_at":     completedAt,
			"static_o_auth_save_proof":            "",
			"static_o_auth_save_proof_hash":       "",
		}
		if status == apitypes.MCPStaticOAuthTestStatusSucceeded {
			saveProof := strings.ToLower(rand.Text())
			encryptedProof, err := c.encryptMCPStaticOAuthSaveProof(ctx, &pending, saveProof)
			if err != nil {
				return fmt.Errorf("failed to encrypt static OAuth save proof: %w", err)
			}
			updates["static_o_auth_save_proof"] = encryptedProof
			updates["static_o_auth_save_proof_hash"] = hashMCPStaticOAuthValue(saveProof)
		}
		return tx.Model(&pending).Updates(updates).Error
	})
}

func validMCPStaticOAuthTestCompletion(status apitypes.MCPStaticOAuthTestStatus, failureCategory apitypes.MCPStaticOAuthTestFailureCategory) bool {
	if status == apitypes.MCPStaticOAuthTestStatusSucceeded {
		return failureCategory == ""
	}
	if status != apitypes.MCPStaticOAuthTestStatusFailed {
		return false
	}
	switch failureCategory {
	case apitypes.MCPStaticOAuthTestFailureAuthorizationDenied,
		apitypes.MCPStaticOAuthTestFailureInvalidCallback,
		apitypes.MCPStaticOAuthTestFailureTokenExchange:
		return true
	default:
		return false
	}
}

type MCPStaticOAuthCredentialClaim struct {
	mcpID        string
	mcpURL       string
	clientID     string
	clientSecret string
	generation   string
}

// ClaimMCPStaticOAuthCredentialProof validates and durably consumes one proof.
// Once the proof row is found, even a rejected Save consumes it. The caller
// must hold the catalog entry's credential lock until the mutation finishes.
func (c *Client) ClaimMCPStaticOAuthCredentialProof(ctx context.Context, state, userID, mcpID, mcpURL, clientID, clientSecret string) (*MCPStaticOAuthCredentialClaim, error) {
	hashedProof := hashMCPStaticOAuthValue(state)
	var rejectedErr error
	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var proof types.MCPOAuthPendingState
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("static_o_auth_save_proof_hash = ?", hashedProof).
			First(&proof).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrMCPStaticOAuthTestInvalid
			}
			return err
		}
		if err := c.decryptMCPOAuthPendingState(ctx, &proof); err != nil {
			return fmt.Errorf("failed to decrypt static OAuth test: %w", err)
		}
		if err := tx.Delete(&proof).Error; err != nil {
			return err
		}
		if !proof.StaticOAuthTest ||
			proof.StaticOAuthTestStatus != apitypes.MCPStaticOAuthTestStatusSucceeded ||
			time.Since(proof.CreatedAt) >= pendingStateTTL ||
			!secureStringEqual(proof.UserID, userID) ||
			!secureStringEqual(proof.MCPID, mcpID) ||
			!secureStringEqual(proof.URL, mcpURL) ||
			!secureStringEqual(proof.ClientID, clientID) ||
			!secureStringEqual(proof.ClientSecret, clientSecret) ||
			!secureStringEqual(proof.StaticOAuthSaveProof, state) {
			rejectedErr = ErrMCPStaticOAuthTestInvalid
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if rejectedErr != nil {
		return nil, rejectedErr
	}
	return &MCPStaticOAuthCredentialClaim{
		mcpID:        mcpID,
		mcpURL:       mcpURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		generation:   hashedProof,
	}, nil
}

// CommitClaimedMCPStaticOAuthCredential applies a previously claimed Save.
// Credential replacement, grant cleanup, and sibling-proof invalidation remain
// atomic even though the submitted proof was consumed before this operation.
func (c *Client) CommitClaimedMCPStaticOAuthCredential(ctx context.Context, claim *MCPStaticOAuthCredentialClaim, replace bool, cleanupMCPIDs ...string) error {
	if claim == nil {
		return ErrMCPStaticOAuthTestInvalid
	}
	if c.encryptionConfig == nil || c.encryptionConfig.Transformers[credentialGroupResource] == nil {
		return ErrMCPStaticOAuthEncryptionRequired
	}
	credential := types.Credential{
		Context: system.MCPOAuthCredentialName(claim.mcpID),
		Name:    "oauth",
		Secrets: map[string]string{
			"CLIENT_ID":     claim.clientID,
			"CLIENT_SECRET": claim.clientSecret,
			"MCP_URL":       claim.mcpURL,
			"GENERATION":    claim.generation,
		},
	}
	if err := c.encryptCredential(ctx, &credential); err != nil {
		return fmt.Errorf("failed to encrypt credential: %w", err)
	}

	var changedMCPIDs []string
	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if replace {
			var existingCredential types.Credential
			if err := tx.Where("context = ? AND name = ?", credential.Context, credential.Name).First(&existingCredential).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrMCPStaticOAuthCredentialNotFound
				}
				return err
			}
			if err := c.decryptCredential(ctx, &existingCredential); err != nil {
				return fmt.Errorf("failed to decrypt existing static OAuth credential: %w", err)
			}
		}

		upsert := clause.OnConflict{
			Columns:   []clause.Column{{Name: "context"}, {Name: "name"}},
			DoUpdates: clause.AssignmentColumns([]string{"secrets", "encrypted"}),
		}
		if !replace {
			upsert.DoNothing = true
			upsert.DoUpdates = nil
		}
		result := tx.Clauses(upsert).Create(&credential)
		if result.Error != nil {
			return result.Error
		}
		if !replace && result.RowsAffected != 1 {
			return ErrMCPStaticOAuthCredentialExists
		}

		if replace {
			var err error
			changedMCPIDs, err = deleteMCPStaticOAuthTokens(tx, claim.mcpID, cleanupMCPIDs)
			if err != nil {
				return err
			}
		}

		result = tx.Delete(
			&types.MCPOAuthPendingState{},
			"mcp_id = ? AND static_o_auth_test = ?",
			claim.mcpID,
			true,
		)
		if result.Error != nil {
			return result.Error
		}
		return nil
	})
	if err != nil {
		return err
	}
	return c.triggerMCPOAuthTokenChanges(ctx, changedMCPIDs)
}

// DeleteMCPStaticOAuthCredential removes the shared application, all pending
// proofs for the entry, and every matching local user grant in one transaction.
// The caller must hold the entry credential lock for the complete operation.
func (c *Client) DeleteMCPStaticOAuthCredential(ctx context.Context, mcpID string, cleanupMCPIDs ...string) (bool, error) {
	return c.deleteMCPStaticOAuthCredential(ctx, mcpID, "", cleanupMCPIDs...)
}

// DeleteMCPStaticOAuthCredentialGeneration removes the shared application only
// when it is still the exact generation reviewed by the caller.
func (c *Client) DeleteMCPStaticOAuthCredentialGeneration(ctx context.Context, mcpID, expectedGeneration string, cleanupMCPIDs ...string) (bool, error) {
	if expectedGeneration == "" {
		return false, ErrMCPOAuthCatalogCredentialChanged
	}
	return c.deleteMCPStaticOAuthCredential(ctx, mcpID, expectedGeneration, cleanupMCPIDs...)
}

func (c *Client) deleteMCPStaticOAuthCredential(ctx context.Context, mcpID, expectedGeneration string, cleanupMCPIDs ...string) (bool, error) {
	credentialContext := system.MCPOAuthCredentialName(mcpID)
	var deleted bool
	var changedMCPIDs []string
	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if expectedGeneration != "" {
			var current types.Credential
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("context = ? AND name = ?", credentialContext, "oauth").
				First(&current).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil {
				if err := c.decryptCredential(ctx, &current); err != nil {
					return fmt.Errorf("failed to decrypt static OAuth credential: %w", err)
				}
				if !secureStringEqual(current.Secrets["GENERATION"], expectedGeneration) {
					return ErrMCPOAuthCatalogCredentialChanged
				}
			}
		}
		result := tx.Where("context = ? AND name = ?", credentialContext, "oauth").Delete(&types.Credential{})
		if result.Error != nil {
			return fmt.Errorf("failed to delete credential: %w", result.Error)
		}
		deleted = result.RowsAffected > 0

		var err error
		changedMCPIDs, err = deleteMCPStaticOAuthTokens(tx, mcpID, cleanupMCPIDs)
		if err != nil {
			return err
		}
		return tx.Where("mcp_id = ? AND static_o_auth_test = ?", mcpID, true).Delete(&types.MCPOAuthPendingState{}).Error
	})
	if err != nil {
		return false, err
	}
	return deleted, c.triggerMCPOAuthTokenChanges(ctx, changedMCPIDs)
}

func deleteMCPStaticOAuthTokens(tx *gorm.DB, catalogEntryName string, cleanupMCPIDs []string) ([]string, error) {
	scope := tx.Model(&types.MCPOAuthToken{}).Where("catalog_entry_name = ?", catalogEntryName)
	if len(cleanupMCPIDs) > 0 {
		scope = scope.Or("mcp_id IN ?", cleanupMCPIDs)
	}
	var mcpIDs []string
	if err := scope.Distinct("mcp_id").Pluck("mcp_id", &mcpIDs).Error; err != nil {
		return nil, err
	}
	if err := scope.Delete(&types.MCPOAuthToken{}).Error; err != nil {
		return nil, err
	}
	return append(mcpIDs, cleanupMCPIDs...), nil
}

func (c *Client) triggerMCPOAuthTokenChanges(ctx context.Context, mcpIDs []string) error {
	var errs []error
	seen := make(map[string]struct{}, len(mcpIDs))
	for _, mcpID := range mcpIDs {
		if mcpID == "" {
			continue
		}
		if _, ok := seen[mcpID]; ok {
			continue
		}
		seen[mcpID] = struct{}{}
		if err := c.triggerMCPOAuthTokenChange(ctx, mcpID); err != nil {
			errs = append(errs, fmt.Errorf("failed to trigger OAuth token change for %s: %w", mcpID, err))
		}
	}
	return errors.Join(errs...)
}

func secureStringEqual(a, b string) bool {
	aHash := sha256.Sum256([]byte(a))
	bHash := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(aHash[:], bHash[:]) == 1
}

func (c *Client) CreateMCPOAuthPendingState(ctx context.Context, userID, mcpID, mcpURL, oauthAuthRequestID, catalogEntryName, state, verifier string, oauthConf *oauth2.Config) error {
	catalogCredentialGeneration := ""
	if catalogEntryName != "" {
		releaseCatalogMutationLock, err := c.AcquireCredentialLock(ctx, system.MCPStaticOAuthCatalogMutationLock)
		if err != nil {
			return fmt.Errorf("failed to coordinate catalog OAuth state with catalog mutation: %w", err)
		}
		defer releaseCatalogMutationLock()
		credentialKey := system.MCPOAuthCredentialName(catalogEntryName)
		releaseCredentialLock, err := c.AcquireCredentialLock(ctx, credentialKey)
		if err != nil {
			return fmt.Errorf("failed to coordinate catalog OAuth state with credential mutation: %w", err)
		}
		defer releaseCredentialLock()
		credential, err := c.RevealCredential(ctx, []string{credentialKey}, "oauth")
		if err != nil || !secureStringEqual(credential.Secrets["CLIENT_ID"], oauthConf.ClientID) || !secureStringEqual(credential.Secrets["CLIENT_SECRET"], oauthConf.ClientSecret) {
			return ErrMCPOAuthCatalogCredentialChanged
		}
		if !secureStringEqual(credential.Secrets["MCP_URL"], mcpURL) {
			return ErrMCPOAuthCatalogCredentialChanged
		}
		catalogCredentialGeneration = credential.Secrets["GENERATION"]
		if catalogCredentialGeneration == "" {
			return ErrMCPOAuthCatalogCredentialChanged
		}
	}
	hashedState := fmt.Sprintf("%x", sha256.Sum256([]byte(state)))
	ps := &types.MCPOAuthPendingState{
		HashedState:                 hashedState,
		State:                       state,
		Verifier:                    verifier,
		UserID:                      userID,
		MCPID:                       mcpID,
		URL:                         mcpURL,
		CatalogEntryName:            catalogEntryName,
		CatalogCredentialGeneration: catalogCredentialGeneration,
		OAuthAuthRequestID:          oauthAuthRequestID,
		ClientID:                    oauthConf.ClientID,
		ClientSecret:                oauthConf.ClientSecret,
		AuthURL:                     oauthConf.Endpoint.AuthURL,
		TokenURL:                    oauthConf.Endpoint.TokenURL,
		AuthStyle:                   oauthConf.Endpoint.AuthStyle,
		RedirectURL:                 oauthConf.RedirectURL,
		Scopes:                      strings.Join(oauthConf.Scopes, " "),
	}

	if err := c.encryptMCPOAuthPendingState(ctx, ps); err != nil {
		return fmt.Errorf("failed to encrypt pending state: %w", err)
	}

	return c.db.WithContext(ctx).Create(ps).Error
}

func (c *Client) GetMCPOAuthPendingState(ctx context.Context, state string) (*types.MCPOAuthPendingState, error) {
	hashedState := fmt.Sprintf("%x", sha256.Sum256([]byte(state)))
	ps := new(types.MCPOAuthPendingState)
	if err := c.db.WithContext(ctx).Where("hashed_state = ?", hashedState).First(ps).Error; err != nil {
		return nil, err
	}

	if err := c.decryptMCPOAuthPendingState(ctx, ps); err != nil {
		return nil, fmt.Errorf("failed to decrypt pending state: %w", err)
	}

	return ps, nil
}

func (c *Client) DeleteMCPOAuthPendingState(ctx context.Context, hashedState string) error {
	return c.db.WithContext(ctx).Delete(&types.MCPOAuthPendingState{}, "hashed_state = ?", hashedState).Error
}

const pendingStateTTL = 30 * time.Minute

func (c *Client) CleanupExpiredMCPOAuthPendingStates(ctx context.Context, olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)
	return c.db.WithContext(ctx).Delete(&types.MCPOAuthPendingState{}, "created_at < ?", cutoff).Error
}

func (c *Client) runPendingStateCleanup(ctx context.Context) {
	timer := time.NewTimer(pendingStateTTL)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		if err := c.CleanupExpiredMCPOAuthPendingStates(ctx, pendingStateTTL); err != nil {
			log.Errorf("Failed to cleanup expired MCP OAuth pending states: %v", err)
		}

		timer.Reset(pendingStateTTL)
	}
}

// Encryption for MCPOAuthToken

func (c *Client) encryptMCPOAuthToken(ctx context.Context, token *types.MCPOAuthToken) error {
	if c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[mcpOAuthTokenGroupResource]
	if transformer == nil {
		return nil
	}

	var (
		b    []byte
		err  error
		errs []error

		dataCtx = mcpOAuthTokenCtx(token)
	)
	if b, err = transformer.TransformToStorage(ctx, []byte(token.AccessToken), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		token.AccessToken = base64.StdEncoding.EncodeToString(b)
	}
	if b, err = transformer.TransformToStorage(ctx, []byte(token.RefreshToken), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		token.RefreshToken = base64.StdEncoding.EncodeToString(b)
	}
	if b, err = transformer.TransformToStorage(ctx, []byte(token.ClientID), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		token.ClientID = base64.StdEncoding.EncodeToString(b)
	}
	if b, err = transformer.TransformToStorage(ctx, []byte(token.ClientSecret), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		token.ClientSecret = base64.StdEncoding.EncodeToString(b)
	}

	token.Encrypted = true

	return errors.Join(errs...)
}

func (c *Client) decryptMCPOAuthToken(ctx context.Context, token *types.MCPOAuthToken) error {
	if !token.Encrypted || c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[mcpOAuthTokenGroupResource]
	if transformer == nil {
		return nil
	}

	var (
		out, decoded []byte
		n            int
		err          error
		errs         []error

		dataCtx = mcpOAuthTokenCtx(token)
	)

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(token.AccessToken)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(token.AccessToken))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			token.AccessToken = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(token.RefreshToken)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(token.RefreshToken))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			token.RefreshToken = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(token.ClientID)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(token.ClientID))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			token.ClientID = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(token.ClientSecret)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(token.ClientSecret))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			token.ClientSecret = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func mcpOAuthTokenCtx(token *types.MCPOAuthToken) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s/%s", mcpOAuthTokenGroupResource.String(), token.MCPID))
}

// Encryption for MCPOAuthPendingState

func (c *Client) encryptMCPOAuthPendingState(ctx context.Context, ps *types.MCPOAuthPendingState) error {
	if c.encryptionConfig == nil {
		if ps.StaticOAuthTest {
			return ErrMCPStaticOAuthEncryptionRequired
		}
		return nil
	}

	transformer := c.encryptionConfig.Transformers[mcpOAuthPendingStateGroupResource]
	if transformer == nil {
		// Fall back to using the token transformer if no specific one is configured
		transformer = c.encryptionConfig.Transformers[mcpOAuthTokenGroupResource]
		if transformer == nil {
			if ps.StaticOAuthTest {
				return ErrMCPStaticOAuthEncryptionRequired
			}
			return nil
		}
	}

	var errs []error
	dataCtx := mcpOAuthPendingStateCtx(ps)
	for _, field := range mcpOAuthPendingStateEncryptedFields(ps) {
		b, err := transformer.TransformToStorage(ctx, []byte(*field), dataCtx)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		*field = base64.StdEncoding.EncodeToString(b)
	}

	ps.Encrypted = true

	return errors.Join(errs...)
}

func (c *Client) decryptMCPOAuthPendingState(ctx context.Context, ps *types.MCPOAuthPendingState) error {
	if !ps.Encrypted {
		return nil
	}
	if c.encryptionConfig == nil {
		return ErrMCPStaticOAuthEncryptionRequired
	}

	transformer := c.encryptionConfig.Transformers[mcpOAuthPendingStateGroupResource]
	if transformer == nil {
		transformer = c.encryptionConfig.Transformers[mcpOAuthTokenGroupResource]
		if transformer == nil {
			return ErrMCPStaticOAuthEncryptionRequired
		}
	}

	var errs []error
	dataCtx := mcpOAuthPendingStateCtx(ps)
	for _, field := range mcpOAuthPendingStateEncryptedFields(ps) {
		decoded, err := base64.StdEncoding.DecodeString(*field)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out, _, err := transformer.TransformFromStorage(ctx, decoded, dataCtx)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		*field = string(out)
	}

	return errors.Join(errs...)
}

func (c *Client) encryptMCPStaticOAuthSaveProof(ctx context.Context, ps *types.MCPOAuthPendingState, proof string) (string, error) {
	if c.encryptionConfig == nil {
		return "", ErrMCPStaticOAuthEncryptionRequired
	}
	transformer := c.encryptionConfig.Transformers[mcpOAuthPendingStateGroupResource]
	if transformer == nil {
		transformer = c.encryptionConfig.Transformers[mcpOAuthTokenGroupResource]
	}
	if transformer == nil {
		return "", ErrMCPStaticOAuthEncryptionRequired
	}
	b, err := transformer.TransformToStorage(ctx, []byte(proof), mcpOAuthPendingStateCtx(ps))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func mcpOAuthPendingStateEncryptedFields(ps *types.MCPOAuthPendingState) []*string {
	fields := []*string{
		&ps.State,
		&ps.Verifier,
		&ps.ClientID,
		&ps.ClientSecret,
	}
	if ps.StaticOAuthTest {
		fields = append(fields, &ps.URL, &ps.AuthURL, &ps.TokenURL, &ps.RedirectURL, &ps.Scopes)
		if ps.StaticOAuthSaveProof != "" {
			fields = append(fields, &ps.StaticOAuthSaveProof)
		}
	}
	return fields
}

func hashMCPStaticOAuthValue(value string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
}

func mcpOAuthPendingStateCtx(ps *types.MCPOAuthPendingState) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s/%s", mcpOAuthPendingStateGroupResource.String(), ps.MCPID))
}
