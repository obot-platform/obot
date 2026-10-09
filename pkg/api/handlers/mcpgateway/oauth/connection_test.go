package oauth

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type oauthConnectionTest struct {
	name          string
	id            string
	legacyAlias   bool
	instanceAlias bool
	allowed       bool
	grantDenied   bool
}

func oauthConnectionTests() []oauthConnectionTest {
	return []oauthConnectionTest{
		{
			name:    "vMCP",
			id:      "vmcp1test",
			allowed: true,
		},
		{
			name:    "vMCP instance",
			id:      "vmcpi1test",
			allowed: true,
		},
		{
			name:        "another user's vMCP instance",
			id:          "vmcpi1foreign",
			allowed:     true,
			grantDenied: true,
		},
		{
			name:        "migrated catalog entry",
			id:          "default-entry",
			legacyAlias: true,
			allowed:     true,
		},
		{
			name:        "migrated server",
			id:          "ms1legacy",
			legacyAlias: true,
			allowed:     true,
		},
		{
			name:        "migrated server instance",
			id:          "msi1legacy",
			legacyAlias: true,
			allowed:     true,
		},
		{
			name:          "migrated dedicated connection",
			id:            "ms1connection",
			instanceAlias: true,
			allowed:       true,
		},
		{
			name:          "migrated shared connection",
			id:            "msi1connection",
			instanceAlias: true,
			allowed:       true,
		},
		{
			name: "catalog entry without vMCP",
			id:   "default-entry",
		},
		{
			name: "standalone server",
			id:   "ms1standalone",
		},
		{
			name: "standalone server instance",
			id:   "msi1standalone",
		},
		{
			name: "component server",
			id:   "ms1component",
		},
	}
}

func oauthConnectionTestObjects(test oauthConnectionTest) []kclient.Object {
	objects := []kclient.Object{
		&v1.VMCPInstance{
			Name:      "vmcpi1foreign",
			Namespace: system.DefaultNamespace,
			Spec: v1.VMCPInstanceSpec{
				UserID:   "43",
				Manifest: types.VMCPInstanceManifest{VMCPID: "vmcp1test"},
			},
		},
		&v1.MCPServerCatalogEntry{
			Name:      "default-entry",
			Namespace: system.DefaultNamespace,
		},
		&v1.MCPServer{
			Name:      "ms1standalone",
			Namespace: system.DefaultNamespace,
			Spec:      v1.MCPServerSpec{UserID: "42"},
		},
		&v1.MCPServerInstance{
			Name:      "msi1standalone",
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerInstanceSpec{
				UserID:        "42",
				MCPServerName: "ms1standalone",
			},
		},
		&v1.MCPServer{
			Name:      "ms1component",
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerSpec{
				UserID:          "42",
				VMCPInstanceID:  "vmcpi1test",
				VMCPComponentID: "component",
			},
		},
	}
	if test.legacyAlias {
		objects = append(objects, &v1.VMCP{
			Name:      "vmcp1migrated",
			Namespace: system.DefaultNamespace,
			Spec: v1.VMCPSpec{
				LegacySlug: test.id,
				Manifest: types.VMCPManifest{
					Components: []types.VMCPComponent{{ID: "component"}},
					Profiles: []types.VMCPProfile{{
						Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
						Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
					}},
				},
			},
		})
	}
	if test.instanceAlias {
		objects = append(objects, &v1.VMCPInstance{
			Name:      "vmcpi1migrated",
			Namespace: system.DefaultNamespace,
			Spec: v1.VMCPInstanceSpec{
				UserID:     "42",
				LegacySlug: test.id,
				Manifest:   types.VMCPInstanceManifest{VMCPID: "vmcp1test"},
			},
		})
	}
	return objects
}

func TestAuthorizeConnectionIDs(t *testing.T) {
	for _, test := range oauthConnectionTests() {
		for _, resourceParameter := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/resource=%v", test.name, resourceParameter), func(t *testing.T) {
				client := &v1.OAuthClient{
					Name:      "client",
					Namespace: system.DefaultNamespace,
					Spec: v1.OAuthClientSpec{Manifest: types.OAuthClientManifest{
						RedirectURIs:            []string{"https://client.example/callback"},
						TokenEndpointAuthMethod: "client_secret_basic",
					}},
				}
				storage := vmcpConsentStorage(append(oauthConnectionTestObjects(test), client,
					&v1.VMCP{Name: "vmcp1test", Namespace: system.DefaultNamespace},
					&v1.VMCPInstance{
						Name:      "vmcpi1test",
						Namespace: system.DefaultNamespace,
						Spec: v1.VMCPInstanceSpec{
							UserID:   "42",
							Manifest: types.VMCPInstanceManifest{VMCPID: "vmcp1test"},
						},
					},
				)...)
				q := url.Values{
					"client_id":     {"default:client"},
					"redirect_uri":  {client.Spec.Manifest.RedirectURIs[0]},
					"response_type": {"code"},
					"state":         {"client-state"},
				}
				if resourceParameter {
					q.Set("resource", "https://obot.example.com/mcp-connect/"+test.id)
				}
				r := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+q.Encode(), nil)
				if !resourceParameter {
					r.SetPathValue("mcp_id", test.id)
				}
				recorder := httptest.NewRecorder()
				h := &handler{oauthConfig: handlers.OAuthAuthorizationServerConfig{ResponseTypesSupported: []string{"code"}}}
				// Authorization must resolve instance aliases before login, when no user is present.
				require.NoError(t, h.authorize(api.Context{Request: r, ResponseWriter: recorder, Storage: storage}))
				location, err := url.Parse(recorder.Header().Get("Location"))
				require.NoError(t, err)
				var requests v1.OAuthAuthRequestList
				require.NoError(t, storage.List(t.Context(), &requests))
				if test.allowed {
					require.Empty(t, location.Query().Get("error"))
					require.Len(t, requests.Items, 1)
					require.Equal(t, test.id, requests.Items[0].Spec.MCPID)
					require.Contains(t, location.Query().Get("rd"), "/oauth/callback/")
				} else {
					require.Equal(t, "invalid_request", location.Query().Get("error"))
					require.Equal(t, "client-state", location.Query().Get("state"))
					require.Empty(t, requests.Items)
				}
			})
		}
	}
}

func TestTokenGrantsConnectionIDs(t *testing.T) {
	for _, test := range oauthConnectionTests() {
		for _, grant := range []string{"authorization_code", "refresh_token"} {
			t.Run(test.name+"/"+grant, func(t *testing.T) {
				const credential = "grant-credential"
				credentialHash := fmt.Sprintf("%x", sha256.Sum256([]byte(credential)))
				resource := "https://obot.example.com/mcp-connect/" + test.id
				client := &v1.OAuthClient{
					Name:      "client",
					Namespace: system.DefaultNamespace,
					Spec: v1.OAuthClientSpec{Manifest: types.OAuthClientManifest{
						TokenEndpointAuthMethod: "none",
						GrantTypes:              []string{"authorization_code", "refresh_token"},
					}},
				}
				objects := append(oauthConnectionTestObjects(test), client)
				form := url.Values{"client_id": {"default:client"}, "grant_type": {grant}}
				if grant == "authorization_code" {
					objects = append(objects, &v1.OAuthAuthRequest{
						Name:      "oar1request",
						Namespace: system.DefaultNamespace,
						Spec: v1.OAuthAuthRequestSpec{
							ClientID:       client.Name,
							MCPID:          test.id,
							Audience:       test.id,
							Resource:       resource,
							UserID:         42,
							HashedAuthCode: credentialHash,
						},
					})
					form.Set("code", credential)
				} else {
					objects = append(objects, &v1.OAuthToken{
						Name:      credentialHash,
						Namespace: system.DefaultNamespace,
						Spec: v1.OAuthTokenSpec{
							ClientID: client.Name,
							MCPID:    test.id,
							Audience: test.id,
							Resource: resource,
							UserID:   42,
						},
					})
					form.Set("refresh_token", credential)
				}
				storage, gateway, tokenService := newOAuthTokenTestServices(t, objects...)
				h := &handler{
					tokenService: tokenService,
					baseURL:      "https://obot.example.com",
					oauthConfig:  handlers.OAuthAuthorizationServerConfig{GrantTypesSupported: []string{"authorization_code", "refresh_token"}},
				}
				r := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				r.SetPathValue("mcp_id", test.id)
				recorder := httptest.NewRecorder()
				req := api.Context{Request: r, ResponseWriter: recorder, Storage: storage, GatewayClient: gateway}
				if !test.allowed {
					require.ErrorContains(t, h.token(req), "invalid_request")
					// Persisted legacy grants must also be rejected on the unscoped endpoint.
					r.SetPathValue("mcp_id", "")
				}
				err := h.token(req)
				var tokens v1.OAuthTokenList
				require.NoError(t, storage.List(t.Context(), &tokens))
				if !test.allowed || test.grantDenied {
					require.ErrorContains(t, err, "invalid_grant")
					require.Empty(t, tokens.Items)
					return
				}
				require.NoError(t, err)
				var response types.OAuthToken
				require.NoError(t, json.NewDecoder(recorder.Body).Decode(&response))
				claims, err := tokenService.DecodeToken(t.Context(), response.AccessToken)
				require.NoError(t, err)
				require.Equal(t, test.id, claims.MCPID)
				require.Len(t, tokens.Items, 1)
				require.Equal(t, test.id, tokens.Items[0].Spec.Audience)
				require.Equal(t, resource, tokens.Items[0].Spec.Resource)
			})
		}
	}
}

func TestOAuthGrantValidatesEveryConnectionReference(t *testing.T) {
	target := &v1.VMCP{
		Name:      "vmcp1migrated",
		Namespace: system.DefaultNamespace,
		Spec:      v1.VMCPSpec{LegacySlug: "default-migrated"},
	}
	storage := vmcpConsentStorage(target)
	req := api.Context{Request: httptest.NewRequest(http.MethodPost, "/oauth/token", nil), Storage: storage}
	for _, test := range []struct {
		name       string
		mcpID      string
		audience   string
		resourceID string
		allowed    bool
	}{
		{
			name:       "canonical and migrated references",
			mcpID:      target.Name,
			audience:   target.Spec.LegacySlug,
			resourceID: target.Spec.LegacySlug,
			allowed:    true,
		},
		{
			name:       "legacy grant without audience",
			mcpID:      target.Spec.LegacySlug,
			resourceID: target.Name,
			allowed:    true,
		},
		{
			name:       "unmigrated MCP ID",
			mcpID:      "default-unmigrated",
			audience:   target.Name,
			resourceID: target.Name,
		},
		{
			name:       "unmigrated audience",
			mcpID:      target.Name,
			audience:   "default-unmigrated",
			resourceID: target.Name,
		},
		{
			name:       "unmigrated resource",
			mcpID:      target.Name,
			audience:   target.Name,
			resourceID: "default-unmigrated",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			allowed, err := validOAuthConnection(req, test.mcpID, test.audience, "https://obot.example.com/mcp-connect/"+test.resourceID, "42")
			require.NoError(t, err)
			require.Equal(t, test.allowed, allowed)
		})
	}
}
