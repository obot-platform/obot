package localauth

import (
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The icons are inlined so that the provider has no external dependencies, unlike the other
// providers whose icons are shipped in the provider registry image.
const (
	icon     = `data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCIgZmlsbD0ibm9uZSIgc3Ryb2tlPSIjMWYyOTM3IiBzdHJva2Utd2lkdGg9IjEuNzUiIHN0cm9rZS1saW5lY2FwPSJyb3VuZCIgc3Ryb2tlLWxpbmVqb2luPSJyb3VuZCI+PHJlY3QgeD0iMyIgeT0iMTAiIHdpZHRoPSIxOCIgaGVpZ2h0PSIxMSIgcng9IjIiLz48cGF0aCBkPSJNNyAxMFY3YTUgNSAwIDAgMSAxMCAwdjMiLz48Y2lyY2xlIGN4PSIxMiIgY3k9IjE1LjUiIHI9IjEuMjUiIGZpbGw9IiMxZjI5MzciIHN0cm9rZT0ibm9uZSIvPjxwYXRoIGQ9Ik0xMiAxNi43NXYxLjc1Ii8+PC9zdmc+`
	iconDark = `data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCIgZmlsbD0ibm9uZSIgc3Ryb2tlPSIjZTVlN2ViIiBzdHJva2Utd2lkdGg9IjEuNzUiIHN0cm9rZS1saW5lY2FwPSJyb3VuZCIgc3Ryb2tlLWxpbmVqb2luPSJyb3VuZCI+PHJlY3QgeD0iMyIgeT0iMTAiIHdpZHRoPSIxOCIgaGVpZ2h0PSIxMSIgcng9IjIiLz48cGF0aCBkPSJNNyAxMFY3YTUgNSAwIDAgMSAxMCAwdjMiLz48Y2lyY2xlIGN4PSIxMiIgY3k9IjE1LjUiIHI9IjEuMjUiIGZpbGw9IiNlNWU3ZWIiIHN0cm9rZT0ibm9uZSIvPjxwYXRoIGQ9Ik0xMiAxNi43NXYxLjc1Ii8+PC9zdmc+`
)

// AuthProvider returns the AuthProvider resource for the built-in local auth provider.
// It has no Command: the dispatcher serves it from within the Obot process instead of launching
// a daemon for it.
func AuthProvider() *v1.AuthProvider {
	return &v1.AuthProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ProviderName,
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: types.AuthProviderManifest{
				CommonProviderMetadata: types.CommonProviderMetadata{
					Name:        "Local",
					Icon:        icon,
					IconDark:    iconDark,
					Description: "Authenticate users with an email address and password stored in Obot. No external identity provider required.",
					RequiredConfigurationParameters: []types.ProviderConfigurationParameter{
						{
							Name:         EmailDomainsEnvVar,
							FriendlyName: "Allowed Email Domains",
							Description:  "Comma-separated list of email domains that local users may have. Use * to allow any domain.",
						},
					},
				},
			},
		},
	}
}
