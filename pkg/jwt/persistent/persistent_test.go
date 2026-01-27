package persistent

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestService(t *testing.T) *TokenService {
	t.Helper()

	// Generate a test key
	_, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	service := &TokenService{
		serverURL: "https://test.obot.ai",
	}

	// Set the key directly for testing
	err = service.replaceKey(context.Background(), privateKey)
	require.NoError(t, err)

	return service
}

func TestNewToken(t *testing.T) {
	service := setupTestService(t)
	ctx := context.Background()

	tests := []struct {
		name        string
		context     TokenContext
		wantErr     bool
		checkClaims func(*testing.T, *TokenContext)
	}{
		{
			name: "basic token with user info",
			context: TokenContext{
				Audience:  "https://test.obot.ai",
				IssuedAt:  time.Now(),
				ExpiresAt: time.Now().Add(time.Hour),
				UserID:    "user-123",
				UserName:  "testuser",
				UserEmail: "test@example.com",
				UserGroups: []string{"group1", "group2"},
			},
			wantErr: false,
			checkClaims: func(t *testing.T, tc *TokenContext) {
				assert.Equal(t, "user-123", tc.UserID)
				assert.Equal(t, "testuser", tc.UserName)
				assert.Equal(t, "test@example.com", tc.UserEmail)
				assert.Equal(t, []string{"group1", "group2"}, tc.UserGroups)
			},
		},
		{
			name: "run token with all fields",
			context: TokenContext{
				Audience:          "https://test.obot.ai",
				IssuedAt:          time.Now(),
				ExpiresAt:         time.Now().Add(time.Hour),
				UserID:            "user-123",
				UserName:          "testuser",
				UserEmail:         "test@example.com",
				TokenType:         TokenTypeRun,
				RunID:             "run-456",
				ThreadID:          "thread-789",
				ProjectID:         "project-abc",
				TopLevelProjectID: "project-top",
				AgentID:           "agent-xyz",
				WorkflowID:        "workflow-123",
				WorkflowStepID:    "step-001",
				Scope:             "run:execute",
				ModelProvider:     "openai",
				Model:             "gpt-4",
				Namespace:         "default",
			},
			wantErr: false,
			checkClaims: func(t *testing.T, tc *TokenContext) {
				assert.Equal(t, TokenTypeRun, tc.TokenType)
				assert.Equal(t, "run-456", tc.RunID)
				assert.Equal(t, "thread-789", tc.ThreadID)
				assert.Equal(t, "project-abc", tc.ProjectID)
				assert.Equal(t, "project-top", tc.TopLevelProjectID)
				assert.Equal(t, "agent-xyz", tc.AgentID)
				assert.Equal(t, "workflow-123", tc.WorkflowID)
				assert.Equal(t, "step-001", tc.WorkflowStepID)
				assert.Equal(t, "run:execute", tc.Scope)
				assert.Equal(t, "openai", tc.ModelProvider)
				assert.Equal(t, "gpt-4", tc.Model)
				assert.Equal(t, "default", tc.Namespace)
			},
		},
		{
			name: "token with base64 picture - should be excluded",
			context: TokenContext{
				Audience:  "https://test.obot.ai",
				IssuedAt:  time.Now(),
				ExpiresAt: time.Now().Add(time.Hour),
				UserID:    "user-123",
				Picture:   "data:image/png;base64,iVBORw0KGgoAAAANS",
			},
			wantErr: false,
			checkClaims: func(t *testing.T, tc *TokenContext) {
				// Picture should be empty because it was a base64 encoded image
				assert.Empty(t, tc.Picture)
			},
		},
		{
			name: "token with URL picture - should be included",
			context: TokenContext{
				Audience:  "https://test.obot.ai",
				IssuedAt:  time.Now(),
				ExpiresAt: time.Now().Add(time.Hour),
				UserID:    "user-123",
				Picture:   "https://example.com/avatar.jpg",
			},
			wantErr: false,
			checkClaims: func(t *testing.T, tc *TokenContext) {
				assert.Equal(t, "https://example.com/avatar.jpg", tc.Picture)
			},
		},
		{
			name: "token with auth provider info",
			context: TokenContext{
				Audience:              "https://test.obot.ai",
				IssuedAt:              time.Now(),
				ExpiresAt:             time.Now().Add(time.Hour),
				UserID:                "user-123",
				AuthProviderName:      "github",
				AuthProviderNamespace: "default",
				AuthProviderUserID:    "github-user-456",
				OAuthScope:            "read:user",
			},
			wantErr: false,
			checkClaims: func(t *testing.T, tc *TokenContext) {
				assert.Equal(t, "github", tc.AuthProviderName)
				assert.Equal(t, "default", tc.AuthProviderNamespace)
				assert.Equal(t, "github-user-456", tc.AuthProviderUserID)
				assert.Equal(t, "read:user", tc.OAuthScope)
			},
		},
		{
			name: "token with MCPID",
			context: TokenContext{
				Audience:  "https://test.obot.ai",
				IssuedAt:  time.Now(),
				ExpiresAt: time.Now().Add(time.Hour),
				UserID:    "user-123",
				MCPID:     "mcp-789",
			},
			wantErr: false,
			checkClaims: func(t *testing.T, tc *TokenContext) {
				assert.Equal(t, "mcp-789", tc.MCPID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create token
			tokenString, err := service.NewToken(ctx, tt.context)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotEmpty(t, tokenString)

			// Decode token to verify
			decoded, err := service.DecodeToken(ctx, tokenString)
			require.NoError(t, err)

			if tt.checkClaims != nil {
				tt.checkClaims(t, decoded)
			}
		})
	}
}

func TestDecodeToken(t *testing.T) {
	service := setupTestService(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		setup   func() string
		wantErr bool
		check   func(*testing.T, *TokenContext)
	}{
		{
			name: "valid token",
			setup: func() string {
				token, err := service.NewToken(ctx, TokenContext{
					Audience:   "https://test.obot.ai",
					IssuedAt:   time.Now(),
					ExpiresAt:  time.Now().Add(time.Hour),
					UserID:     "user-123",
					UserName:   "testuser",
					UserEmail:  "test@example.com",
					UserGroups: []string{"admin", "users"},
				})
				require.NoError(t, err)
				return token
			},
			wantErr: false,
			check: func(t *testing.T, tc *TokenContext) {
				assert.Equal(t, "user-123", tc.UserID)
				assert.Equal(t, "testuser", tc.UserName)
				assert.Equal(t, "test@example.com", tc.UserEmail)
				assert.Equal(t, []string{"admin", "users"}, tc.UserGroups)
			},
		},
		{
			name: "token with empty groups",
			setup: func() string {
				token, err := service.NewToken(ctx, TokenContext{
					Audience:   "https://test.obot.ai",
					IssuedAt:   time.Now(),
					ExpiresAt:  time.Now().Add(time.Hour),
					UserID:     "user-123",
					UserGroups: []string{},
				})
				require.NoError(t, err)
				return token
			},
			wantErr: false,
			check: func(t *testing.T, tc *TokenContext) {
				assert.Empty(t, tc.UserGroups)
			},
		},
		{
			name: "invalid token format",
			setup: func() string {
				return "invalid.token.format"
			},
			wantErr: true,
		},
		{
			name: "empty token",
			setup: func() string {
				return ""
			},
			wantErr: true,
		},
		{
			name: "token with wrong signature",
			setup: func() string {
				// Create a token with a different key
				_, wrongKey, err := ed25519.GenerateKey(nil)
				require.NoError(t, err)

				claims := jwt.MapClaims{
					"iss": "https://test.obot.ai",
					"aud": "https://test.obot.ai",
					"sub": "user-123",
					"exp": float64(time.Now().Add(time.Hour).Unix()),
					"iat": float64(time.Now().Unix()),
				}

				token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
				tokenString, err := token.SignedString(wrongKey)
				require.NoError(t, err)
				return tokenString
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenString := tt.setup()

			decoded, err := service.DecodeToken(ctx, tokenString)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, decoded)

			if tt.check != nil {
				tt.check(t, decoded)
			}
		})
	}
}

func TestDecodeToken_BackwardsCompatibility(t *testing.T) {
	service := setupTestService(t)
	ctx := context.Background()

	// Test backwards compatibility for UserName and UserEmail
	// These fields changed from "UserName"/"UserEmail" to "name"/"email"
	// The decoder should prefer the new fields but fall back to old ones
	t.Run("old field names", func(t *testing.T) {
		claims := jwt.MapClaims{
			"iss":      "https://test.obot.ai",
			"aud":      "https://test.obot.ai",
			"sub":      "user-123",
			"exp":      float64(time.Now().Add(time.Hour).Unix()),
			"iat":      float64(time.Now().Unix()),
			"UserName": "oldname",
			"UserEmail": "old@example.com",
		}

		_, tokenString, err := service.NewTokenWithClaims(ctx, claims)
		require.NoError(t, err)

		decoded, err := service.DecodeToken(ctx, tokenString)
		require.NoError(t, err)
		assert.Equal(t, "oldname", decoded.UserName)
		assert.Equal(t, "old@example.com", decoded.UserEmail)
	})

	t.Run("new field names override old", func(t *testing.T) {
		claims := jwt.MapClaims{
			"iss":       "https://test.obot.ai",
			"aud":       "https://test.obot.ai",
			"sub":       "user-123",
			"exp":       float64(time.Now().Add(time.Hour).Unix()),
			"iat":       float64(time.Now().Unix()),
			"name":      "newname",
			"email":     "new@example.com",
			"UserName":  "oldname",
			"UserEmail": "old@example.com",
		}

		_, tokenString, err := service.NewTokenWithClaims(ctx, claims)
		require.NoError(t, err)

		decoded, err := service.DecodeToken(ctx, tokenString)
		require.NoError(t, err)
		// New names should take precedence
		assert.Equal(t, "newname", decoded.UserName)
		assert.Equal(t, "new@example.com", decoded.UserEmail)
	})
}

func TestDecodeToken_GroupsParsing(t *testing.T) {
	service := setupTestService(t)
	ctx := context.Background()

	tests := []struct {
		name          string
		groupsString  string
		expectedSlice []string
	}{
		{
			name:          "multiple groups",
			groupsString:  "admin,users,developers",
			expectedSlice: []string{"admin", "users", "developers"},
		},
		{
			name:          "single group",
			groupsString:  "admin",
			expectedSlice: []string{"admin"},
		},
		{
			name:          "empty groups",
			groupsString:  "",
			expectedSlice: []string{},
		},
		{
			name:          "groups with empty elements",
			groupsString:  "admin,,users,,",
			expectedSlice: []string{"admin", "users"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := jwt.MapClaims{
				"iss":        "https://test.obot.ai",
				"aud":        "https://test.obot.ai",
				"sub":        "user-123",
				"exp":        float64(time.Now().Add(time.Hour).Unix()),
				"iat":        float64(time.Now().Unix()),
				"UserGroups": tt.groupsString,
			}

			_, tokenString, err := service.NewTokenWithClaims(ctx, claims)
			require.NoError(t, err)

			decoded, err := service.DecodeToken(ctx, tokenString)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedSlice, decoded.UserGroups)
		})
	}
}

func TestAuthenticateRequest(t *testing.T) {
	service := setupTestService(t)
	ctx := context.Background()

	tests := []struct {
		name       string
		setup      func() *http.Request
		wantAuth   bool
		wantOK     bool
		checkUser  func(*testing.T, *http.Request, interface{})
	}{
		{
			name: "valid run token",
			setup: func() *http.Request {
				token, err := service.NewToken(ctx, TokenContext{
					Audience:          "https://test.obot.ai",
					IssuedAt:          time.Now(),
					ExpiresAt:         time.Now().Add(time.Hour),
					UserID:            "user-123",
					UserName:          "testuser",
					UserEmail:         "test@example.com",
					TokenType:         TokenTypeRun,
					RunID:             "run-456",
					ThreadID:          "thread-789",
					ProjectID:         "project-abc",
					TopLevelProjectID: "project-top",
					AgentID:           "agent-xyz",
					Scope:             "run:execute",
					UserGroups:        []string{"group1"},
				})
				require.NoError(t, err)

				req, _ := http.NewRequest("GET", "https://test.obot.ai", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				return req
			},
			wantAuth: true,
			wantOK:   true,
			checkUser: func(t *testing.T, req *http.Request, user interface{}) {
				// For run tokens, the name should be the Scope
				// and extra fields should contain run-specific info
				// No additional validation needed here
			},
		},
		{
			name: "valid regular token",
			setup: func() *http.Request {
				token, err := service.NewToken(ctx, TokenContext{
					Audience:              "https://test.obot.ai",
					IssuedAt:              time.Now(),
					ExpiresAt:             time.Now().Add(time.Hour),
					UserID:                "user-123",
					UserName:              "testuser",
					UserEmail:             "test@example.com",
					UserGroups:            []string{"admin"},
					AuthProviderName:      "github",
					AuthProviderNamespace: "default",
					MCPID:                 "mcp-789",
					OAuthScope:            "read:user",
				})
				require.NoError(t, err)

				req, _ := http.NewRequest("GET", "https://test.obot.ai", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				return req
			},
			wantAuth: true,
			wantOK:   true,
		},
		{
			name: "no authorization header",
			setup: func() *http.Request {
				req, _ := http.NewRequest("GET", "https://test.obot.ai", nil)
				return req
			},
			wantAuth: false,
			wantOK:   false,
		},
		{
			name: "empty bearer token",
			setup: func() *http.Request {
				req, _ := http.NewRequest("GET", "https://test.obot.ai", nil)
				req.Header.Set("Authorization", "Bearer ")
				return req
			},
			wantAuth: false,
			wantOK:   false,
		},
		{
			name: "invalid token format",
			setup: func() *http.Request {
				req, _ := http.NewRequest("GET", "https://test.obot.ai", nil)
				req.Header.Set("Authorization", "Bearer invalid.token.format")
				return req
			},
			wantAuth: false,
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setup()

			resp, ok, err := service.AuthenticateRequest(req)

			if tt.wantAuth {
				require.NoError(t, err)
				require.True(t, ok)
				require.NotNil(t, resp)
				require.NotNil(t, resp.User)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantOK, ok)
				if !tt.wantOK {
					assert.Nil(t, resp)
				}
			}

			if tt.checkUser != nil && resp != nil {
				tt.checkUser(t, req, resp)
			}
		})
	}
}

func TestNewTokenWithClaims(t *testing.T) {
	service := setupTestService(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		claims  jwt.MapClaims
		wantErr bool
		check   func(*testing.T, *jwt.Token, string)
	}{
		{
			name: "custom claims with issuer override",
			claims: jwt.MapClaims{
				"sub":    "user-123",
				"custom": "value",
				"aud":    "", // Empty string should be replaced
			},
			wantErr: false,
			check: func(t *testing.T, token *jwt.Token, tokenString string) {
				claims, ok := token.Claims.(jwt.MapClaims)
				require.True(t, ok)
				// Issuer should be set by the service
				assert.Equal(t, "https://test.obot.ai", claims["iss"])
				// Audience should be set to serverURL if empty
				assert.Equal(t, "https://test.obot.ai", claims["aud"])
				assert.Equal(t, "user-123", claims["sub"])
				assert.Equal(t, "value", claims["custom"])
			},
		},
		{
			name: "custom claims with explicit audience",
			claims: jwt.MapClaims{
				"sub": "user-123",
				"aud": "custom-audience",
			},
			wantErr: false,
			check: func(t *testing.T, token *jwt.Token, tokenString string) {
				claims, ok := token.Claims.(jwt.MapClaims)
				require.True(t, ok)
				// Explicit audience should be preserved
				assert.Equal(t, "custom-audience", claims["aud"])
			},
		},
		{
			name: "empty claims",
			claims: jwt.MapClaims{
				"aud": "", // Empty string should be replaced
			},
			wantErr: false,
			check: func(t *testing.T, token *jwt.Token, tokenString string) {
				claims, ok := token.Claims.(jwt.MapClaims)
				require.True(t, ok)
				// Issuer and audience should still be set
				assert.Equal(t, "https://test.obot.ai", claims["iss"])
				assert.Equal(t, "https://test.obot.ai", claims["aud"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, tokenString, err := service.NewTokenWithClaims(ctx, tt.claims)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, token)
			assert.NotEmpty(t, tokenString)

			if tt.check != nil {
				tt.check(t, token, tokenString)
			}
		})
	}
}

func TestReplaceKey(t *testing.T) {
	service := setupTestService(t)
	ctx := context.Background()

	// Create a token with the original key
	originalToken, err := service.NewToken(ctx, TokenContext{
		Audience:  "https://test.obot.ai",
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
		UserID:    "user-123",
	})
	require.NoError(t, err)

	// Replace the key
	_, newKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	err = service.replaceKey(ctx, newKey)
	require.NoError(t, err)

	// Original token should no longer be valid
	_, err = service.DecodeToken(ctx, originalToken)
	assert.Error(t, err)

	// New tokens should work
	newToken, err := service.NewToken(ctx, TokenContext{
		Audience:  "https://test.obot.ai",
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
		UserID:    "user-456",
	})
	require.NoError(t, err)

	decoded, err := service.DecodeToken(ctx, newToken)
	require.NoError(t, err)
	assert.Equal(t, "user-456", decoded.UserID)
}


func TestNewTokenService(t *testing.T) {
	var gatewayClient *client.Client
	var credOnlyGPTClient *gptscript.GPTScript

	service, err := NewTokenService("https://test.obot.ai", gatewayClient, credOnlyGPTClient)
	require.NoError(t, err)
	require.NotNil(t, service)
	assert.Equal(t, "https://test.obot.ai", service.serverURL)
}

func TestTokenContext_Timestamps(t *testing.T) {
	service := setupTestService(t)
	ctx := context.Background()

	now := time.Now()
	expiresAt := now.Add(2 * time.Hour)

	tokenString, err := service.NewToken(ctx, TokenContext{
		Audience:  "https://test.obot.ai",
		IssuedAt:  now,
		ExpiresAt: expiresAt,
		UserID:    "user-123",
	})
	require.NoError(t, err)

	decoded, err := service.DecodeToken(ctx, tokenString)
	require.NoError(t, err)

	// Timestamps should be preserved (within a second due to Unix timestamp precision)
	assert.WithinDuration(t, now, decoded.IssuedAt, time.Second)
	assert.WithinDuration(t, expiresAt, decoded.ExpiresAt, time.Second)
}

func TestBase64EncodedKey(t *testing.T) {
	// Test that keys can be properly encoded/decoded
	_, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	encoded := base64.StdEncoding.EncodeToString(privateKey)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)

	assert.Equal(t, privateKey, ed25519.PrivateKey(decoded))
}
