package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/authentication/user"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBasicUserCanRestartOwnedNanobotMCPServer(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	server := &v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ms1agent",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MCPServerSpec{
			UserID:         "user-1",
			NanobotAgentID: "nba1agent",
		},
	}

	client := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(server).Build()
	authorizer := NewAuthorizer(client, client, false, nil, false)
	req := httptest.NewRequest(http.MethodPost, "/api/mcp-servers/ms1agent/restart", nil)
	u := &user.DefaultInfo{UID: "user-1", Groups: []string{types.GroupBasic, types.GroupAuthenticated}}

	if !authorizer.Authorize(req, u) {
		t.Fatal("expected basic user to be authorized to restart owned nanobot MCP server")
	}
}

func TestBasicUserCannotRestartAnotherUsersNanobotMCPServer(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	server := &v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ms1agent",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MCPServerSpec{
			UserID:         "owner-1",
			NanobotAgentID: "nba1agent",
		},
	}

	client := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(server).Build()
	authorizer := NewAuthorizer(client, client, false, nil, false)
	req := httptest.NewRequest(http.MethodPost, "/api/mcp-servers/ms1agent/restart", nil)
	u := &user.DefaultInfo{UID: "user-1", Groups: []string{types.GroupBasic, types.GroupAuthenticated}}

	if authorizer.Authorize(req, u) {
		t.Fatal("expected basic user to be denied for another user's nanobot MCP server")
	}
}

func TestBasicUserCanLaunchOwnedNanobotAgent(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	project := &v1.ProjectV2{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pv21project",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.ProjectV2Spec{
			UserID: "user-1",
		},
	}
	agent := &v1.NanobotAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nba1agent",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.NanobotAgentSpec{
			UserID:      "user-1",
			ProjectV2ID: project.Name,
		},
	}

	client := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(project, agent).Build()
	authorizer := NewAuthorizer(client, client, false, nil, false)
	req := httptest.NewRequest(http.MethodPost, "/api/projectsv2/pv21project/agents/nba1agent/launch", nil)
	u := &user.DefaultInfo{UID: "user-1", Groups: []string{types.GroupBasic, types.GroupAuthenticated}}

	if !authorizer.Authorize(req, u) {
		t.Fatal("expected basic user to be authorized to launch owned nanobot agent")
	}
}

func TestBasicUserCannotLaunchAnotherUsersNanobotAgent(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	project := &v1.ProjectV2{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pv21project",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.ProjectV2Spec{
			UserID: "owner-1",
		},
	}
	agent := &v1.NanobotAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nba1agent",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.NanobotAgentSpec{
			UserID:      "owner-1",
			ProjectV2ID: project.Name,
		},
	}

	client := clientfake.NewClientBuilder().WithScheme(scheme).WithObjects(project, agent).Build()
	authorizer := NewAuthorizer(client, client, false, nil, false)
	req := httptest.NewRequest(http.MethodPost, "/api/projectsv2/pv21project/agents/nba1agent/launch", nil)
	u := &user.DefaultInfo{UID: "user-1", Groups: []string{types.GroupBasic, types.GroupAuthenticated}}

	if authorizer.Authorize(req, u) {
		t.Fatal("expected basic user to be denied for another user's nanobot agent")
	}
}
