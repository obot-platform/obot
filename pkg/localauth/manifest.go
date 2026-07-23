package localauth

import (
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The icons are inlined so that the provider has no external dependencies, unlike the other
// providers whose icons are shipped in the provider registry image.
// Sourced from ui/user/static/user/images/obot-icon-blue.svg (visible path only).
const (
	icon     = `data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyMjcuMTUgMjA4LjM4Ij48cGF0aCBmaWxsPSIjNEY3RUYzIiBmaWxsLXJ1bGU9ImV2ZW5vZGQiIGNsaXAtcnVsZT0iZXZlbm9kZCIgZD0iTTExMy41NCwwLjg5Yy03LjE1LDAtMTMuMDMsNS44OS0xMy4wMywxMy4wM2wwLDBjMC4wMSw0Ljc1LDIuNjMsOS4xNSw2LjgxLDExLjQydjMwLjQ4SDYyLjg0Yy0xNi41NSwwLTMxLjA0LDguODctMzguOTksMjIuMDZjLTAuNTgtMC4xLTEuMTYtMC4xNi0xLjc0LTAuMjJjLTEyLjEsMC0yMi4wNiw5Ljk2LTIyLjA2LDIyLjA2bDAsMGMwLDEyLjExLDkuOTYsMjIuMDYsMjIuMDYsMjIuMDZjMC4xNC0wLjAxLDAuMjgtMC4wMiwwLjQyLTAuMDNjNy41OSwxNC40NSwyMi44MywyNC4zNCw0MC4zMiwyNC4zNGgxMDEuNGMxNy40OSwwLDMyLjczLTkuODksNDAuMzItMjQuMzVjMC4xNCwwLjAxLDAuMjgsMC4wMywwLjQyLDAuMDRjMTIuMSwwLDIyLjA2LTkuOTYsMjIuMDYtMjIuMDZsMCwwYzAtMTIuMTEtOS45Ni0yMi4wNi0yMi4wNi0yMi4wNmMtMC41OCwwLjA1LTEuMTcsMC4xMi0xLjc0LDAuMjJjLTcuOTUtMTMuMi0yMi40NC0yMi4wNy0zOC45OS0yMi4wN2gtNDQuNDhWMjUuMzRjNC4xOC0yLjI4LDYuOC02LjY2LDYuODEtMTEuNDJsMCwwQzEyNi41Nyw2Ljc3LDEyMC42OSwwLjg5LDExMy41NCwwLjg5TDExMy41NCwwLjg5eiBNNjIuODQsNzAuMWgxOS4xYy0wLjAyLDAuMjctMC4wNCwwLjU0LTAuMDQsMC44MmMwLDYuNDUsNS4xOSwxMS42NCwxMS42NSwxMS42NGgzOS45OWM2LjQ1LDAsMTEuNjUtNS4xOSwxMS42NS0xMS42NGMwLTAuMjgtMC4wMi0wLjU1LTAuMDQtMC44MmgxOS4xYzE3LjQyLDAsMzEuMTYsMTMuNjYsMzEuMTYsMzAuODVjMCwxNy4xOC0xMy43NSwzMC44NS0zMS4xNiwzMC44NUg2Mi44NGMtMTcuNDIsMC0zMS4xNi0xMy42Ny0zMS4xNi0zMC44NUMzMS42Nyw4My43Niw0NS40Miw3MC4xLDYyLjg0LDcwLjFMNjIuODQsNzAuMXogTTYwLjczLDg1LjY1TDYwLjczLDg1LjY1Yy04LjM5LDAtMTUuMyw2LjktMTUuMywxNS4zbDAsMGMwLDguMzksNi45MSwxNS4zLDE1LjMsMTUuM2wwLDBjOC4zOSwwLDE1LjMtNi45MSwxNS4zLTE1LjNsMCwwQzc2LjAzLDkyLjU1LDY5LjEyLDg1LjY1LDYwLjczLDg1LjY1eiBNMTY2LjM1LDg1LjY1Yy04LjM5LDAtMTUuMyw2LjktMTUuMywxNS4zbDAsMGMwLDguMzksNi45MSwxNS4zLDE1LjMsMTUuM2wwLDBjOC4zOSwwLDE1LjMtNi45MSwxNS4zLTE1LjNsMCwwQzE4MS42NSw5Mi41NSwxNzQuNzQsODUuNjUsMTY2LjM1LDg1LjY1TDE2Ni4zNSw4NS42NXogTTkyLjkxLDk0LjgyYzAsMTEuMzksOS4yMywyMC42MywyMC42MiwyMC42M2MxMS4zOSwwLDIwLjYyLTkuMjMsMjAuNjItMjAuNjNIOTIuOTF6IE01OS43MywxNTcuMDFjLTAuNTIsMi45OS0wLjgyLDYuMDYtMC44Miw5LjJ2MTUuMDdjLTExLjA0LDQuMDQtMTguOTMsMTQuNi0xOC45MywyNy4wM0g5Ny42YzAtMTIuNTctOC4wNy0yMy4yMy0xOS4zLTI3LjE2di0xNC45NGMwLTMuMTUsMC40MS02LjE5LDEuMTctOS4wOEg2Mi44NEM2MS43OSwxNTcuMTMsNjAuNzYsMTU3LjA4LDU5LjczLDE1Ny4wMXogTTE2Ny4zNCwxNTcuMDFjLTEuMDMsMC4wOC0yLjA2LDAuMTItMy4xLDAuMTJIMTQ3LjZjMC43NiwyLjg5LDEuMTcsNS45MywxLjE3LDkuMDh2MTQuOTRjLTExLjIzLDMuOTMtMTkuMywxNC41OS0xOS4zLDI3LjE2aDU3LjYxYzAtMTIuNDMtNy44OS0yMi45OS0xOC45My0yNy4wMnYtMTUuMDhDMTY4LjE2LDE2My4wNywxNjcuODYsMTYwLDE2Ny4zNCwxNTcuMDF6Ii8+PC9zdmc+`
	iconDark = icon
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
