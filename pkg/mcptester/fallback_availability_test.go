package mcptester

import (
	"context"
	"errors"
	"net/url"
	"testing"
)

type availabilityProviders struct {
	configured bool
	err        error
}

type availabilitySettings struct {
	enabled bool
	err     error
}

func (p availabilityProviders) HasModelProvider(context.Context) (bool, error) {
	return p.configured, p.err
}

func (s availabilitySettings) ModelProxyEnabled(context.Context) (bool, error) {
	return s.enabled, s.err
}

func TestResolveFallbackAvailability(t *testing.T) {
	endpoint := &url.URL{Scheme: "https", Host: "model.example"}
	lookupErr := errors.New("configuration changing")

	for _, tt := range []struct {
		name         string
		endpoint     *url.URL
		providers    ProviderConfigurationResolver
		settings     ModelProxySettingsReader
		wantProvider any
		wantEnabled  bool
		wantErr      bool
	}{
		{
			name:         "enabled without providers",
			endpoint:     endpoint,
			providers:    availabilityProviders{},
			settings:     availabilitySettings{enabled: true},
			wantProvider: false,
			wantEnabled:  true,
		},
		{
			name:         "empty URL disables fallback",
			providers:    availabilityProviders{},
			settings:     availabilitySettings{enabled: true},
			wantProvider: false,
		},
		{
			name:         "admin switch disables fallback",
			endpoint:     endpoint,
			providers:    availabilityProviders{},
			settings:     availabilitySettings{},
			wantProvider: false,
		},
		{
			name:         "configured provider bypasses proxy settings",
			endpoint:     endpoint,
			providers:    availabilityProviders{configured: true},
			settings:     availabilitySettings{err: lookupErr},
			wantProvider: true,
		},
		{
			name:      "pending provider configuration is unknown",
			endpoint:  endpoint,
			providers: availabilityProviders{err: lookupErr},
			settings:  availabilitySettings{enabled: true},
			wantErr:   true,
		},
		{
			name:     "missing resolver is unknown",
			endpoint: endpoint,
			settings: availabilitySettings{enabled: true},
			wantErr:  true,
		},
		{
			name:      "settings failure does not permit fallback",
			endpoint:  endpoint,
			providers: availabilityProviders{},
			settings: availabilitySettings{
				enabled: true,
				err:     lookupErr,
			},
			wantProvider: false,
			wantErr:      true,
		},
		{
			name:         "missing settings do not permit fallback",
			endpoint:     endpoint,
			providers:    availabilityProviders{},
			wantProvider: false,
			wantErr:      true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveFallbackAvailability(t.Context(), tt.endpoint, tt.providers, tt.settings)

			var provider any
			if got.HasModelProvider != nil {
				provider = *got.HasModelProvider
			}

			if provider != tt.wantProvider || got.Enabled != tt.wantEnabled || (err != nil) != tt.wantErr {
				t.Fatalf("provider=%v enabled=%v err=%v", provider, got.Enabled, err)
			}
		})
	}
}
