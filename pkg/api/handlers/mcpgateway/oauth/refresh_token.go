package oauth

import (
	"cmp"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	refreshTokenReuseWindow = time.Minute
)

func invalidRefreshToken() error {
	return types.NewErrBadRequest("%v", newOAuthError(ErrInvalidGrant, "refresh_token is invalid", ""))
}

func refreshTokenHash(token string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
}

func refreshTokenName(token string) (string, error) {
	if !strings.Contains(token, ".") {
		// Legacy tokens locate their grant by hash, including after its first in-place rotation.
		return refreshTokenHash(token), nil
	}

	// Versioned tokens carry a stable grant ID; validRefreshToken still authenticates
	// the full bearer token. Keep a fourth part to reject extra separators without
	// unbounded allocation.
	parts := strings.SplitN(token, ".", 4)
	if len(parts) != 3 || parts[0] != "v1" || len(parts[1]) != 64 || len(parts[2]) != 43 || parts[1] != strings.ToLower(parts[1]) {
		return "", invalidRefreshToken()
	}
	if _, err := hex.DecodeString(parts[1]); err != nil {
		return "", invalidRefreshToken()
	}
	if _, err := base64.RawURLEncoding.DecodeString(parts[2]); err != nil {
		return "", invalidRefreshToken()
	}
	return parts[1], nil
}

func validRefreshToken(token *v1.OAuthToken, presented string) bool {
	hash := refreshTokenHash(presented)
	// Only unrotated legacy grants use their record name as the current hash.
	// After rotation, retaining that name must not bypass the previous token's deadline.
	return hash == cmp.Or(token.Spec.CurrentTokenHash, token.Name) ||
		hash == token.Spec.PreviousTokenHash && time.Now().Before(token.Spec.PreviousValidUntil.Time)
}

func refreshTokenCipher(token *v1.OAuthToken, previous string) (cipher.AEAD, []byte, error) {
	// Any replica can derive this key when the client retries with the previous token.
	// Use the actual bearer secret, never its stored hash, so storage alone cannot recover a successor.
	key, err := hkdf.Key(sha256.New, []byte(previous), []byte(token.UID), "obot/mcp-refresh-retry/v1", 32)
	if err != nil {
		return nil, nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, nil, err
	}

	// Authenticate the grant identity and rotation metadata so ciphertext cannot be
	// moved to another grant or given a different retry deadline.
	aad, err := json.Marshal([]string{
		token.Namespace, token.Name, string(token.UID), token.Spec.ClientID,
		token.Spec.CurrentTokenHash, token.Spec.PreviousTokenHash,
		token.Spec.PreviousValidUntil.UTC().Format(time.RFC3339),
	})
	return aead, aad, err
}

// recoverRefreshToken requires the caller to authenticate the predecessor and check
// its deadline first; the encrypted recovery value may remain stored after expiry.
func recoverRefreshToken(token *v1.OAuthToken, previous string) (string, error) {
	aead, aad, err := refreshTokenCipher(token, previous)
	if err != nil {
		return "", err
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(token.Spec.EncryptedCurrentToken)
	if err != nil {
		return "", fmt.Errorf("failed to decode refresh token recovery data: %w", err)
	}
	plaintext, err := aead.Open(nil, nil, ciphertext, aad)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt refresh token recovery data: %w", err)
	}
	if refreshTokenHash(string(plaintext)) != token.Spec.CurrentTokenHash {
		return "", fmt.Errorf("recovered refresh token hash does not match")
	}
	return string(plaintext), nil
}

func rotateRefreshToken(req api.Context, token *v1.OAuthToken, presented string) (string, error) {
	// Authorization and access-token minting may have outlasted the retry window.
	// Check the presented token again before using its recovery data.
	if !validRefreshToken(token, presented) {
		return "", invalidRefreshToken()
	}
	if refreshTokenHash(presented) == token.Spec.PreviousTokenHash {
		return recoverRefreshToken(token, presented)
	}
	// Keep both accepted tokens on the same generation until the fixed deadline.
	// Rotating the current token now would strand requests still retrying its predecessor.
	if time.Now().Before(token.Spec.PreviousValidUntil.Time) {
		return presented, nil
	}

	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	// All generations keep the original record name, so deleting it revokes the grant.
	next := "v1." + token.Name + "." + base64.RawURLEncoding.EncodeToString(secret)
	token.Spec.CurrentTokenHash = refreshTokenHash(next)
	token.Spec.PreviousTokenHash = refreshTokenHash(presented)
	// metav1.Time persists whole seconds, so use that precision for the authenticated deadline too.
	token.Spec.PreviousValidUntil = metav1.NewTime(time.Now().UTC().Truncate(time.Second).Add(refreshTokenReuseWindow))
	aead, aad, err := refreshTokenCipher(token, presented)
	if err != nil {
		return "", err
	}
	// shortcut: retain one expired ciphertext until the next rotation or grant deletion; add timed scrubbing if required.
	token.Spec.EncryptedCurrentToken = base64.RawStdEncoding.EncodeToString(aead.Seal(nil, nil, []byte(next), aad))

	// Storage rejects stale resourceVersions and deleted records, so only one
	// successor is committed and refresh cannot recreate a revoked grant.
	if err := req.Storage.Update(req.Context(), token); err != nil {
		// An update can commit even when its response is lost. Only return the committed successor.
		var latest v1.OAuthToken
		if getErr := req.Storage.Get(req.Context(), kclient.ObjectKeyFromObject(token), &latest); apierrors.IsNotFound(getErr) {
			return "", invalidRefreshToken()
		} else if getErr != nil {
			return "", fmt.Errorf("failed to rotate oauth token: %w", err)
		}
		// A record recreated under the same name is a different grant.
		if latest.UID != token.UID {
			return "", invalidRefreshToken()
		}
		// A different stored hash does not confirm this candidate; preserve the update
		// error so a conflict retries with fresh state rather than returning an uncommitted token.
		if latest.Spec.CurrentTokenHash != token.Spec.CurrentTokenHash {
			return "", err
		}
		*token = latest
	}
	return next, nil
}
