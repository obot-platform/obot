package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/cmd"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/spf13/cobra"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestRegistryAccessIsNotConfiguredByEnvironment(t *testing.T) {
	t.Setenv("OBOT_SERVER_ENABLE_ANONYMOUS_REGISTRY_ACCESS", "true")
	t.Setenv("OBOT_SERVER_ENABLE_REGISTRY_AUTH", "false")
	server := &Server{}
	command := cmd.Command(&Obot{}, server)
	command.PersistentPreRunE = nil
	serverCommand := command.Commands()[0]
	serverCommand.RunE = nil
	serverCommand.Run = func(_ *cobra.Command, _ []string) {}
	command.SetArgs([]string{"server"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if serverCommand.Flags().Lookup("enable-anonymous-registry-access") != nil {
		t.Fatal("obsolete flag remains available")
	}
	a := authz.NewAuthorizer(nil, nil, nil, false, nil, nil, nil)
	if a.Authorize(httptest.NewRequest(http.MethodGet, "/v0.1/servers", nil), &user.DefaultInfo{Groups: []string{authz.UnauthenticatedGroup}}) {
		t.Fatal("obsolete environment enabled anonymous access")
	}
	command.SetArgs([]string{"server", "--enable-anonymous-registry-access=true"})
	if err := command.Execute(); err == nil {
		t.Fatal("obsolete flag accepted")
	}
}
