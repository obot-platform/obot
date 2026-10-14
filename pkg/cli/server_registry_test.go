package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/cmd"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/spf13/cobra"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestAnonymousRegistryAccessConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name             string
		env              string
		legacyEnv        string
		args             []string
		anonymousAllowed bool
	}{
		{
			name: "default requires authentication",
		},
		{
			name: "explicitly disable anonymous access",
			env:  "false",
		},
		{
			name:             "anonymous discovery opt-in",
			env:              "true",
			anonymousAllowed: true,
		},
		{
			name:             "flag can opt into anonymous discovery",
			env:              "false",
			args:             []string{"--enable-anonymous-registry-access=true"},
			anonymousAllowed: true,
		},
		{
			name: "flag can disable anonymous discovery",
			env:  "true",
			args: []string{"--enable-anonymous-registry-access=false"},
		},
		{
			name:      "obsolete setting does not enable anonymous access",
			legacyEnv: "false",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OBOT_SERVER_ENABLE_ANONYMOUS_REGISTRY_ACCESS", tc.env)
			t.Setenv("OBOT_SERVER_ENABLE_REGISTRY_AUTH", tc.legacyEnv)
			server := &Server{}
			command := cmd.Command(&Obot{}, server)
			command.PersistentPreRunE = nil
			serverCommand := command.Commands()[0]
			serverCommand.RunE = nil
			serverCommand.Run = func(_ *cobra.Command, _ []string) {}
			command.SetArgs(append([]string{"server"}, tc.args...))
			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			if server.EnableAnonymousRegistryAccess != tc.anonymousAllowed {
				t.Fatalf("EnableAnonymousRegistryAccess = %v, want %v", server.EnableAnonymousRegistryAccess, tc.anonymousAllowed)
			}

			authorizer := authz.NewAuthorizer(nil, nil, nil, false, nil, nil, server.EnableAnonymousRegistryAccess)
			anonymous := &user.DefaultInfo{UID: "anonymous", Groups: []string{authz.UnauthenticatedGroup}}
			authenticated := &user.DefaultInfo{UID: "user-1", Groups: []string{types.GroupBasic, types.GroupAuthenticated}}
			for _, path := range []string{
				"/v0.1/servers",
				"/v0.1/servers/com.example%2Fvmcp1example/versions",
				"/v0.1/servers/com.example%2Fvmcp1example/versions/latest",
			} {
				req := httptest.NewRequest(http.MethodGet, path, nil)
				if allowed := authorizer.Authorize(req, anonymous); allowed != tc.anonymousAllowed {
					t.Fatalf("anonymous access to %s = %v, want %v", path, allowed, tc.anonymousAllowed)
				}
				if !authorizer.Authorize(req, authenticated) {
					t.Fatalf("authenticated access to %s must remain available", path)
				}
			}
		})
	}
}
