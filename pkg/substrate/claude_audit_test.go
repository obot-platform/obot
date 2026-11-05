package substrate

import (
	"context"
	"encoding/json"
	"testing"

	gt "github.com/obot-platform/obot/pkg/gateway/types"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	pb "github.com/obot-platform/obot/pkg/substrate/ateapipb"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type auditControl struct {
	pb.ControlClient
	template *pb.ActorTemplate
	policy   *pb.EgressPolicy
	actor    *pb.Actor
	updates  int
}

func (c *auditControl) CreateAtespace(_ context.Context, r *pb.CreateAtespaceRequest, _ ...grpc.CallOption) (*pb.Atespace, error) {
	return r.Atespace, nil
}
func (c *auditControl) GetActor(context.Context, *pb.GetActorRequest, ...grpc.CallOption) (*pb.Actor, error) {
	if c.actor != nil {
		return c.actor, nil
	}
	return nil, status.Error(codes.NotFound, "missing")
}
func (c *auditControl) GetActorTemplate(context.Context, *pb.GetActorTemplateRequest, ...grpc.CallOption) (*pb.ActorTemplate, error) {
	return nil, status.Error(codes.NotFound, "missing")
}
func (c *auditControl) CreateActorTemplate(_ context.Context, r *pb.CreateActorTemplateRequest, _ ...grpc.CallOption) (*pb.ActorTemplate, error) {
	c.template = r.ActorTemplate
	return r.ActorTemplate, nil
}
func (c *auditControl) CreateActor(_ context.Context, r *pb.CreateActorRequest, _ ...grpc.CallOption) (*pb.Actor, error) {
	return r.Actor, nil
}
func (c *auditControl) CreateActorEgressPolicy(_ context.Context, r *pb.CreateActorEgressPolicyRequest, _ ...grpc.CallOption) (*pb.EgressPolicy, error) {
	c.policy = r.EgressPolicy
	return r.EgressPolicy, nil
}

func TestClaudeAuditTrustAndPolicy(t *testing.T) {
	control := &auditControl{}
	client := &Client{Control: control, Config: Config{ObotURL: "http://obot.obot.svc:8080"}}
	inst := &v1.AgentInstance{Spec: v1.AgentInstanceSpec{UserID: "7", ClaudeEnvironment: "env", ClaudeRuntime: `{"image":"runner@sha256:abc","egressHosts":["*.github.com"],"managedSettings":{"permissions":{"deny":["Read(./secret)"]}}}`}}
	inst.Name = "acr-test"
	_, err := client.EnsureClaude(t.Context(), inst, map[string]string{"token": "token", "workOrder": "order"}, []string{"*.github.com"}, false, nil)
	require.NoError(t, err)
	require.Len(t, control.policy.Rules, 2)
	require.Nil(t, control.policy.Rules[1].TlsPassthrough)
	require.Equal(t, []string{"api.anthropic.com", "*.github.com"}, control.policy.Rules[1].Https.Hostnames)
	var trust *pb.TrustBundleDataSource
	for _, volume := range control.template.Volumes {
		for _, source := range volume.GetSystemInfo().GetDataSources() {
			if source.TrustBundle != nil {
				trust = source.TrustBundle
			}
		}
	}
	require.NotNil(t, trust)
	require.Equal(t, []string{"egress-mitm.ate.dev"}, trust.Names)
	env := map[string]string{}
	for _, variable := range control.template.Containers[0].Env {
		env[variable.Name] = variable.Value
	}
	require.Equal(t, "/run/actor/"+trust.Path, env["NODE_EXTRA_CA_CERTS"])
	require.Equal(t, env["NODE_EXTRA_CA_CERTS"], env["SSL_CERT_FILE"])
	require.Equal(t, env["SSL_CERT_FILE"], env["GIT_SSL_CAINFO"])
	require.NotContains(t, env, "NODE_TLS_REJECT_UNAUTHORIZED")
	require.JSONEq(t, `{"permissions":{"deny":["Read(./secret)"]}}`, env["OBOT_CLAUDE_MANAGED_SETTINGS"])
}

func (c *auditControl) GetActorEgressPolicy(context.Context, *pb.GetActorEgressPolicyRequest, ...grpc.CallOption) (*pb.EgressPolicy, error) {
	if c.policy == nil {
		return nil, status.Error(codes.NotFound, "missing")
	}
	return c.policy, nil
}

func (c *auditControl) UpdateActorEgressPolicy(_ context.Context, r *pb.UpdateActorEgressPolicyRequest, _ ...grpc.CallOption) (*pb.EgressPolicy, error) {
	c.updates++
	c.policy = r.EgressPolicy
	return c.policy, nil
}

func TestClaudeRunningActorEgressUpdates(t *testing.T) {
	control := &auditControl{}
	client := &Client{Control: control, Config: Config{ObotURL: "http://obot.obot.svc:8080"}}
	instance := &v1.AgentInstance{Spec: v1.AgentInstanceSpec{
		UserID:            "7",
		ClaudeEnvironment: "env",
		ClaudeRuntime:     `{"image":"runner@sha256:abc","egressHosts":["thedadams.com"]}`,
	}}
	instance.Name = "acr-test"
	actor, err := client.EnsureClaude(t.Context(), instance, nil, []string{"thedadams.com"}, false, nil)
	require.NoError(t, err)
	actor.Status = &pb.ActorStatus{State: pb.ActorState_ACTOR_STATE_RUNNING}
	control.actor = actor
	control.policy.Metadata.Uid = "policy-uid"
	control.policy.Metadata.Version = 7

	// Current environment settings supersede the execution's saved hosts.
	_, err = client.EnsureClaude(t.Context(), instance, nil, []string{"*.github.com"}, false, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"api.anthropic.com", "*.github.com"}, control.policy.Rules[1].Https.Hostnames)
	require.Equal(t, "policy-uid", control.policy.Metadata.Uid)
	require.Equal(t, int64(7), control.policy.Metadata.Version)
	require.Equal(t, pb.ActorState_ACTOR_STATE_RUNNING, actor.Status.State)
	require.Equal(t, 1, control.updates)

	_, err = client.EnsureClaude(t.Context(), instance, nil, []string{"*.github.com"}, false, nil)
	require.NoError(t, err)
	require.Equal(t, 1, control.updates)

	_, err = client.EnsureClaude(t.Context(), instance, nil, nil, false, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"api.anthropic.com"}, control.policy.Rules[1].Https.Hostnames)
	require.Equal(t, []string{"obot.obot.svc"}, control.policy.Rules[0].Http.Hostnames)
	require.Equal(t, 2, control.updates)
}

func TestClaudeAllowAllEgress(t *testing.T) {
	control := &auditControl{}
	client := &Client{Control: control, Config: Config{ObotURL: "http://obot.obot.svc:8080"}}
	instance := &v1.AgentInstance{Spec: v1.AgentInstanceSpec{
		UserID:            "7",
		ClaudeEnvironment: "env",
		ClaudeRuntime:     `{"image":"runner@sha256:abc"}`,
	}}
	instance.Name = "acr-test"
	actor, err := client.EnsureClaude(t.Context(), instance, nil, nil, false, nil)
	require.NoError(t, err)
	actor.Status = &pb.ActorStatus{State: pb.ActorState_ACTOR_STATE_RUNNING}
	control.actor = actor

	_, err = client.EnsureClaude(t.Context(), instance, nil, nil, true, nil)
	require.NoError(t, err)
	require.Len(t, control.policy.Rules, 3)
	require.Equal(t, []string{"obot.obot.svc"}, control.policy.Rules[0].Http.Hostnames)
	require.Len(t, control.policy.Rules[0].Http.Effects.ReplaceHeaders, 1)
	require.Equal(t, []string{"*"}, control.policy.Rules[1].Http.Hostnames)
	// Wildcard HTTP and HTTPS rules must use distinct ports.
	require.Nil(t, control.policy.Rules[1].Http.Ports.All)
	require.Equal(t, []int32{80}, control.policy.Rules[1].Http.Ports.Numbers)
	require.Nil(t, control.policy.Rules[1].Http.Effects)
	require.Equal(t, []string{"*"}, control.policy.Rules[2].Https.Hostnames)
	require.Nil(t, control.policy.Rules[2].Https.Ports.All)
	require.Equal(t, []int32{443}, control.policy.Rules[2].Https.Ports.Numbers)
	require.Nil(t, control.policy.Rules[2].Https.Effects)
	require.Nil(t, control.policy.Rules[2].TlsPassthrough)
	require.Equal(t, 1, control.updates)

	_, err = client.EnsureClaude(t.Context(), instance, nil, nil, true, nil)
	require.NoError(t, err)
	require.Equal(t, 1, control.updates)

	_, err = client.EnsureClaude(t.Context(), instance, nil, []string{"*.github.com"}, false, nil)
	require.NoError(t, err)
	require.Len(t, control.policy.Rules, 2)
	require.Equal(t, []string{"api.anthropic.com", "*.github.com"}, control.policy.Rules[1].Https.Hostnames)
	require.Equal(t, []int32{443}, control.policy.Rules[1].Https.Ports.Numbers)
	require.Equal(t, 2, control.updates)
	require.Equal(t, pb.ActorState_ACTOR_STATE_RUNNING, actor.Status.State)
}

func TestClaudeGenericSecretPolicy(t *testing.T) {
	control := &auditControl{}
	client := &Client{Control: control, Config: Config{ObotURL: "http://obot.obot.svc:8080"}}
	instance := &v1.AgentInstance{Spec: v1.AgentInstanceSpec{UserID: "7", ClaudeEnvironment: "env", ClaudeRuntime: `{}`}}
	instance.Name = "acr-test"
	secrets := []gt.ClaudeEgressSecret{
		{ID: "ces-token", Hostname: "api.example.com", Header: "authorization", Value: "private-value"},
		{ID: "ces-header", Hostname: "api.example.com", Header: "x-api-key", Value: "private-key"},
		{ID: "ces-denied", Hostname: "denied.example.org", Header: "authorization", Value: "denied-key"},
	}
	_, err := client.EnsureClaude(t.Context(), instance, nil, []string{"*.example.com", "api.example.com"}, false, secrets)
	require.NoError(t, err)
	require.Len(t, control.policy.Rules, 3)
	require.Equal(t, []string{"api.anthropic.com", "*.example.com"}, control.policy.Rules[1].Https.Hostnames)
	injected := control.policy.Rules[2].Https
	require.Equal(t, []string{"api.example.com"}, injected.Hostnames)
	require.Equal(t, []int32{443}, injected.Ports.Numbers)
	require.Len(t, injected.Effects.ReplaceHeaders, 2)
	require.Equal(t, "ate-secret://obot/claude/acr-test/ces-token", injected.Effects.ReplaceHeaders[0].CredentialUri)
	raw, err := json.Marshal([]any{control.policy, control.template})
	require.NoError(t, err)
	require.NotContains(t, string(raw), "private-value")
	require.NotContains(t, string(raw), "private-key")
	require.NotContains(t, string(raw), "ces-denied")
	_, err = client.EnsureClaude(t.Context(), instance, nil, nil, true, secrets)
	require.NoError(t, err)
	require.Len(t, control.policy.Rules, 5)
	_, err = client.EnsureClaude(t.Context(), instance, nil, nil, false, secrets)
	require.NoError(t, err)
	require.Len(t, control.policy.Rules, 2)
}
