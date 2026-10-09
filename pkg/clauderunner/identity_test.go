package clauderunner

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestClaudeTagSessionIdentity(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	verifier := Verifier{keys: map[string]*ecdsa.PublicKey{"test": &key.PublicKey}, fetched: time.Now()}
	for _, test := range []struct {
		name    string
		account string
		actor   string
		owner   string
	}{
		{name: "human", account: "user_abc", actor: "user:user_abc", owner: "user_abc"},
		{name: "service", actor: "agent:tag-agent", owner: ClaudeTagAccount},
		{name: "missing identity"},
		{name: "human without account", actor: "user:user_abc"},
		{name: "service with human account", account: "user_abc", actor: "agent:tag-agent"},
		{name: "empty service", actor: "agent:"},
		{name: "unknown actor", actor: "other:tag-agent"},
	} {
		t.Run(test.name, func(t *testing.T) {
			claims := jwt.MapClaims{
				"iss":            "ccr",
				"sub":            "ccr:session:cse_test",
				"aud":            []string{"ccpool_test"},
				"exp":            time.Now().Add(time.Hour).Unix(),
				"ccr:role":       "session_worker",
				"ccr:pool_id":    "ccpool_test",
				"ccr:org_id":     "org_test",
				"ccr:session_id": "cse_test",
				"ccr:account_id": test.account,
				"act":            map[string]string{"sub": test.actor},
			}
			token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
			token.Header["kid"] = "test"
			signed, err := token.SignedString(key)
			require.NoError(t, err)
			verified, err := verifier.Verify(t.Context(), "sk-ant-cc-"+signed, "ccpool_test", "org_test")
			if test.owner == "" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			owner, err := verified.Owner()
			require.NoError(t, err)
			require.Equal(t, test.owner, owner)
			_, err = verifier.Verify(t.Context(), "sk-ant-cc-"+signed, "ccpool_other", "org_test")
			require.Error(t, err)
		})
	}
}
