package authz

import (
	"net/http/httptest"
	"testing"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCheckMCPIDAllowsAnonymousMCPConnect(t *testing.T) {
	authorizer := &Authorizer{}
	req := httptest.NewRequest("GET", "/mcp-connect/ms1test", nil)

	ok, err := authorizer.checkMCPID(req, &Resources{MCPID: "ms1test"}, &user.DefaultInfo{Name: "anonymous"})
	if err != nil {
		t.Fatalf("checkMCPID() error = %v", err)
	}
	if !ok {
		t.Fatal("checkMCPID() = false, want true")
	}
}

func TestCheckMCPIDDoesNotBypassNonMCPConnectForAnonymous(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()
	authorizer := &Authorizer{cache: storage, uncached: storage}
	req := httptest.NewRequest("GET", "/oauth/authorize/ms1test", nil)

	ok, err := authorizer.checkMCPID(req, &Resources{MCPID: "ms1test"}, &user.DefaultInfo{Name: "anonymous"})
	if err == nil {
		t.Fatal("checkMCPID() error = nil, want error")
	}
	if ok {
		t.Fatal("checkMCPID() = true, want false")
	}
}

func TestCheckMCPIDChecksMCPServerInstanceOwner(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.MCPServerInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "msi1test",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MCPServerInstanceSpec{
			UserID: "user-uid",
		},
	}).Build()
	authorizer := &Authorizer{cache: storage, uncached: storage}
	req := httptest.NewRequest("GET", "/mcp-connect/msi1test", nil)

	ok, err := authorizer.checkMCPID(req, &Resources{MCPID: "msi1test"}, &user.DefaultInfo{
		Name: "user",
		UID:  "user-uid",
	})
	if err != nil {
		t.Fatalf("checkMCPID() error = %v", err)
	}
	if !ok {
		t.Fatal("checkMCPID() = false, want true")
	}
}
