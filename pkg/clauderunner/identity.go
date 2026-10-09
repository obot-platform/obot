// Package clauderunner verifies identities issued to Claude self-hosted sessions.
package clauderunner

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ClaudeTagAccount identifies accountless service sessions within an environment.
const (
	ClaudeTagAccount = "claude-tag"
)

var (
	claudeServiceActor = regexp.MustCompile(`^agent:[a-zA-Z0-9_-]{1,128}$`)
	claudeAccountID    = regexp.MustCompile(`^user_01[1-9A-HJ-NP-Za-km-z]{22}$`)
	claudeAccountUUID  = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

type Claims struct {
	jwt.RegisteredClaims
	Role    string `json:"ccr:role"`
	Pool    string `json:"ccr:pool_id"`
	Org     string `json:"ccr:org_id"`
	OrgUUID string `json:"organization_uuid"`
	Account string `json:"ccr:account_id"`
	Session string `json:"ccr:session_id"`
	Actor   struct {
		Subject string `json:"sub"`
		Email   string `json:"email"`
	} `json:"act"`
}

type Verifier struct {
	mu      sync.Mutex
	keys    map[string]*ecdsa.PublicKey
	fetched time.Time
}

// Owner distinguishes service identities explicitly; a missing account alone is
// never sufficient to authorize a Claude Tag session.
func (c *Claims) Owner() (string, error) {
	if c.Account != "" && strings.HasPrefix(c.Account, "user_") && c.Actor.Subject == "user:"+c.Account {
		return c.Account, nil
	}
	if c.Account == "" && claudeServiceActor.MatchString(c.Actor.Subject) {
		return ClaudeTagAccount, nil
	}
	return "", fmt.Errorf("claude identity account mismatch")
}

// NormalizeAccountID accepts the UUID saved by Claude Code login or the tagged ID
// used in work orders. Claude Code 2.1.289 encodes UUIDs as user_01 plus 22 base58 digits.
func NormalizeAccountID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if claudeAccountID.MatchString(value) {
		return value, nil
	}
	if !claudeAccountUUID.MatchString(value) {
		return "", fmt.Errorf("enter your Claude account UUID or user_ account ID, not an email or organization ID")
	}
	raw, err := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	if err != nil {
		return "", err
	}

	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	encoded := []byte(strings.Repeat("1", 22))
	number, base, remainder := new(big.Int).SetBytes(raw), big.NewInt(58), new(big.Int)
	for i := len(encoded) - 1; number.Sign() > 0; i-- {
		number.QuoRem(number, base, remainder)
		encoded[i] = alphabet[remainder.Int64()]
	}
	return "user_01" + string(encoded), nil
}

func (v *Verifier) Verify(ctx context.Context, raw, pool, org string) (*Claims, error) {
	if !strings.HasPrefix(raw, "sk-ant-cc-") || len(raw) > 32768 {
		return nil, fmt.Errorf("invalid Claude session token")
	}
	claims := new(Claims)
	_, err := jwt.ParseWithClaims(strings.TrimPrefix(raw, "sk-ant-cc-"), claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, fmt.Errorf("missing signing key")
		}
		return v.key(ctx, kid)
	}, jwt.WithValidMethods([]string{"ES256"}), jwt.WithIssuer("ccr"), jwt.WithAudience(pool), jwt.WithExpirationRequired())
	if err != nil {
		reason := "signature or signing key"
		switch {
		case errors.Is(err, jwt.ErrTokenExpired):
			reason = "expired"
		case errors.Is(err, jwt.ErrTokenInvalidAudience):
			reason = "audience"
		case errors.Is(err, jwt.ErrTokenInvalidIssuer):
			reason = "issuer"
		case errors.Is(err, jwt.ErrTokenMalformed):
			reason = "format"
		}
		return nil, fmt.Errorf("invalid Claude session token: %s", reason)
	}
	// Claude signs both the tagged organization ID and its UUID. An environment
	// may be configured with either value.
	switch {
	case claims.Pool != pool:
		return nil, fmt.Errorf("claude identity pool mismatch")
	case claims.Org != org && claims.OrgUUID != org:
		return nil, fmt.Errorf("claude identity organization mismatch")
	case claims.Role != "session_worker":
		return nil, fmt.Errorf("claude identity role mismatch")
	case claims.Session == "" || claims.Subject != "ccr:session:"+claims.Session:
		return nil, fmt.Errorf("claude identity session mismatch")
	}
	if _, err := claims.Owner(); err != nil {
		return nil, err
	}
	return claims, nil
}

func (v *Verifier) key(ctx context.Context, kid string) (*ecdsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if key := v.keys[kid]; key != nil && time.Since(v.fetched) < 5*time.Minute {
		return key, nil
	}
	// Bound refreshes for unknown kids, including requests carrying attacker-chosen kids.
	if time.Since(v.fetched) < 10*time.Second {
		return nil, fmt.Errorf("unknown signing key")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.anthropic.com/v1/code/.well-known/jwks.json", nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("claude signing keys unavailable")
	}
	var set struct {
		Keys []struct{ Kty, Crv, Kid, X, Y string }
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&set); err != nil {
		return nil, err
	}
	keys := map[string]*ecdsa.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "EC" || k.Crv != "P-256" {
			continue
		}
		x, e1 := base64.RawURLEncoding.DecodeString(k.X)
		y, e2 := base64.RawURLEncoding.DecodeString(k.Y)
		if e1 != nil || e2 != nil || len(x) != 32 || len(y) != 32 {
			continue
		}
		point := append(append([]byte{4}, x...), y...)
		key, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), point)
		if err == nil {
			keys[k.Kid] = key
		}
	}
	v.keys, v.fetched = keys, time.Now()
	if key := keys[kid]; key != nil {
		return key, nil
	}
	return nil, fmt.Errorf("unknown signing key")
}
