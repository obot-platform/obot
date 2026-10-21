package substrate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/obot-platform/obot/pkg/clauderunner"
	gt "github.com/obot-platform/obot/pkg/gateway/types"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	pb "github.com/obot-platform/obot/pkg/substrate/ateapipb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	// llmKeyPlaceholder is the runner's X-Obot-Api-Key value until egress replaces it.
	llmKeyPlaceholder = "obot-egress-placeholder"
)

func (c *Client) EnsureClaude(ctx context.Context, instance *v1.AgentInstance, secrets map[string]string, egressHosts []string, allowAllEgress bool, egressSecrets []gt.ClaudeEgressSecret) (*pb.Actor, error) {
	var runtime gt.ClaudeRuntime
	if err := json.Unmarshal([]byte(instance.Spec.ClaudeRuntime), &runtime); err != nil {
		return nil, fmt.Errorf("invalid session runtime: %w", err)
	}
	ref := Ref(instance)
	_, err := c.Control.CreateAtespace(ctx, &pb.CreateAtespaceRequest{Atespace: &pb.Atespace{Metadata: &pb.ResourceMetadata{Name: ref.Atespace}}})
	if err != nil && status.Code(err) != codes.AlreadyExists {
		return nil, err
	}

	actor, actorErr := c.Control.GetActor(ctx, &pb.GetActorRequest{Actor: ref})
	if actorErr != nil && status.Code(actorErr) != codes.NotFound {
		return nil, actorErr
	}
	var source *pb.ObjectRef
	sourceTemplate := ref
	if status.Code(actorErr) == codes.NotFound && instance.Spec.ClaudePrevious != "" {
		source = &pb.ObjectRef{Atespace: ref.Atespace, Name: instance.Spec.ClaudePrevious}
		previous, err := c.Control.GetActor(ctx, &pb.GetActorRequest{Actor: source})
		if err != nil {
			return nil, err
		}
		if previous.GetStatus().GetState() != pb.ActorState_ACTOR_STATE_SUSPENDED {
			return nil, fmt.Errorf("waiting for previous session checkpoint")
		}
		sourceTemplate = previous.GetActorTemplate()
		_, err = c.Control.CreateTag(ctx, &pb.CreateTagRequest{Tag: &pb.Tag{Metadata: &pb.ResourceMetadata{Atespace: source.Atespace, Name: source.Name}, Scope: pb.TagScope_TAG_SCOPE_ATESPACE, SourceActor: source}})
		if err != nil && status.Code(err) != codes.AlreadyExists {
			return nil, err
		}
	}

	_, err = c.Control.GetActorTemplate(ctx, &pb.GetActorTemplateRequest{ActorTemplate: ref})
	if status.Code(err) == codes.NotFound {
		_, err = c.Control.CreateActorTemplate(ctx, &pb.CreateActorTemplateRequest{ActorTemplate: &pb.ActorTemplate{
			Metadata:       &pb.ResourceMetadata{Atespace: ref.Atespace, Name: ref.Name},
			WorkerSelector: &pb.Selector{MatchLabels: map[string]string{"obot.ai/claude-environment": instance.Spec.ClaudeEnvironment}},
			SandboxConfig:  &pb.SandboxConfig{SandboxClass: pb.SandboxClass_SANDBOX_CLASS_GVISOR, ConfigName: "gvisor-default"},
			Resources:      &pb.Resources{Limits: []*pb.Limits{{Name: "cpu", Quantity: runtime.Resources.ActorCPU}, {Name: "memory", Quantity: runtime.Resources.ActorMemory}}},
			SnapshotConfig: &pb.SnapshotConfig{OnPause: pb.SnapshotContentScope_SNAPSHOT_CONTENT_SCOPE_DATA, OnCommit: pb.SnapshotContentScope_SNAPSHOT_CONTENT_SCOPE_DATA, StorageLocation: c.Config.SnapshotLocation},
			Volumes:        []*pb.Volume{{Name: "workspace", DurableDir: &pb.DurableDirVolumeSource{}}, {Name: "identity", SystemInfo: &pb.SystemInfoVolumeSource{DataSources: []*pb.SystemInfoDataSource{{TrustBundle: &pb.TrustBundleDataSource{Names: []string{"egress-mitm.ate.dev"}, Path: "egress-ca.pem"}}, {ActorMetadata: &pb.ActorMetadataDataSource{Items: []*pb.ActorMetadataItem{{Field: pb.ActorMetadataField_ACTOR_METADATA_FIELD_NAME, Path: "name"}}}}}}}},
			Containers: []*pb.Container{{Name: "claude", Image: runtime.Image,
				VolumeMounts: []*pb.VolumeMount{{Name: "workspace", MountPath: "/workspace"}, {Name: "identity", MountPath: "/run/actor"}},
				WakeupProbe:  &pb.ContainerWakeupProbe{HttpGet: &pb.HTTPGetAction{Path: "/healthz", Port: 80}, TimeoutSeconds: 120},
				Env:          append([]*pb.EnvVar{{Name: "OBOT_CLAUDE_MANAGED_SETTINGS", Value: string(runtime.ManagedSettings)}, {Name: "NODE_EXTRA_CA_CERTS", Value: "/run/actor/egress-ca.pem"}, {Name: "SSL_CERT_FILE", Value: "/run/actor/egress-ca.pem"}, {Name: "REQUESTS_CA_BUNDLE", Value: "/run/actor/egress-ca.pem"}, {Name: "GIT_SSL_CAINFO", Value: "/run/actor/egress-ca.pem"}, {Name: "CURL_CA_BUNDLE", Value: "/run/actor/egress-ca.pem"}, {Name: "OBOT_CLAUDE_IDLE_TIMEOUT_MINUTES", Value: strconv.Itoa(runtime.Resources.IdleTimeoutMinutes)}, {Name: "OBOT_URL", Value: c.Config.ObotURL}, {Name: "OBOT_INSTANCE", Value: instance.Name}, {Name: "OBOT_RUNNER_TOKEN", Value: secrets["token"]}, {Name: "OBOT_WORK_ORDER", Value: secrets["workOrder"]}}, llmProxyEnv(runtime, c.Config.ObotURL)...),
			}},
		}})
	}
	if err != nil && status.Code(err) != codes.AlreadyExists {
		return nil, err
	}
	if status.Code(actorErr) == codes.NotFound {
		actor, err = c.Control.CreateActor(ctx, &pb.CreateActorRequest{Actor: &pb.Actor{Metadata: &pb.ResourceMetadata{Atespace: ref.Atespace, Name: ref.Name}, ActorTemplate: sourceTemplate, SourceTag: source}})
	}
	if err != nil {
		return nil, err
	}

	// Tags can only create actors using the template that produced the snapshot.
	// Repoint the suspended clone to this attempt's compatible template before
	// resuming, so it receives the new single-use work order and credentials.
	// Reconcile this separately to recover if Obot stopped after CreateActor.
	if !proto.Equal(actor.GetActorTemplate(), ref) {
		actor.ActorTemplate = ref
		actor, err = c.Control.UpdateActor(ctx, &pb.UpdateActorRequest{Actor: actor})
		if err != nil {
			return nil, err
		}
	}

	obot, _ := url.Parse(c.Config.ObotURL)
	port := int32(80)
	if obot.Port() != "" {
		n, _ := strconv.Atoi(obot.Port())
		port = int32(n)
	}
	rule := &pb.EgressRule{Http: &pb.HTTPRule{
		Hostnames: []string{obot.Hostname()},
		Ports:     &pb.Ports{Numbers: []int32{port}},
		Effects: &pb.HttpRuleEffects{ReplaceHeaders: []*pb.CredentialHeader{{
			Header:        "X-Obot-Egress-Key",
			Prefix:        "Bearer ",
			CredentialUri: "ate-secret://obot/claude/" + instance.Name,
		}}},
	}}
	if runtime.LLMProxy {
		rule.Http.Effects.ReplaceHeaders = append(rule.Http.Effects.ReplaceHeaders, &pb.CredentialHeader{
			// Replaces llmKeyPlaceholder, so the runner never holds its LLM proxy key.
			Header:        "X-Obot-Api-Key",
			CredentialUri: "ate-secret://obot/claude/" + instance.Name + "/llm",
		})
	}
	hosts := []string{"api.anthropic.com"}
	for _, host := range egressHosts {
		if host = strings.TrimSpace(host); host != "" {
			hosts = append(hosts, host)
		}
	}
	desired := &pb.EgressPolicy{
		Metadata: &pb.ResourceMetadata{Atespace: ref.Atespace, Name: "default"},
		Rules:    []*pb.EgressRule{rule, {Https: &pb.HTTPSRule{Hostnames: hosts, Ports: &pb.Ports{Numbers: []int32{443}}}}},
	}
	if allowAllEgress {
		// Keep the specific Obot rule so only Obot receives injected credentials.
		// Separate HTTP and HTTPS ports avoid a protocol tie.
		desired.Rules = []*pb.EgressRule{
			rule,
			{Http: &pb.HTTPRule{Hostnames: []string{"*"}, Ports: &pb.Ports{Numbers: []int32{80}}}},
			{Https: &pb.HTTPSRule{Hostnames: []string{"*"}, Ports: &pb.Ports{Numbers: []int32{443}}}},
		}
	}

	// Exact-host rules take precedence over wildcard rules, while retaining HTTPS inspection.
	injected := map[string][]*pb.CredentialHeader{}
	currentRuntime := gt.ClaudeRuntime{EgressHosts: egressHosts, AllowAllEgress: allowAllEgress}
	for _, secret := range egressSecrets {
		if clauderunner.EgressHostAllowed(currentRuntime, secret.Hostname) {
			injected[secret.Hostname] = append(injected[secret.Hostname], &pb.CredentialHeader{
				Header: secret.Header, CredentialUri: "ate-secret://obot/claude/" + instance.Name + "/" + secret.ID,
			})
		}
	}
	destinations := make([]string, 0, len(injected))
	for host := range injected {
		destinations = append(destinations, host)
	}
	sort.Strings(destinations)
	// Replace overlapping exact rules so policy ranking is unambiguous.
	if !allowAllEgress {
		filtered := hosts[:0]
		for _, host := range hosts {
			if _, ok := injected[host]; !ok {
				filtered = append(filtered, host)
			}
		}
		if len(filtered) == 0 {
			desired.Rules = desired.Rules[:1]
		} else {
			desired.Rules[1].Https.Hostnames = filtered
		}
	}
	for _, host := range destinations {
		desired.Rules = append(desired.Rules, &pb.EgressRule{Https: &pb.HTTPSRule{
			Hostnames: []string{host}, Ports: &pb.Ports{Numbers: []int32{443}},
			Effects: &pb.HttpRuleEffects{ReplaceHeaders: injected[host]},
		}})
	}

	current, err := c.Control.GetActorEgressPolicy(ctx, &pb.GetActorEgressPolicyRequest{Actor: ref})
	if status.Code(err) == codes.NotFound {
		_, err = c.Control.CreateActorEgressPolicy(ctx, &pb.CreateActorEgressPolicyRequest{Actor: ref, EgressPolicy: desired})
		if status.Code(err) == codes.AlreadyExists {
			// Another reconciler created it; the next pass checks its contents.
			err = nil
		}
	} else if err == nil {
		// Retain the UID/version preconditions and avoid writes when unchanged.
		desired.Metadata = current.Metadata
		if !proto.Equal(current, desired) {
			_, err = c.Control.UpdateActorEgressPolicy(ctx, &pb.UpdateActorEgressPolicyRequest{Actor: ref, EgressPolicy: desired})
		}
	}
	if err != nil {
		return nil, err
	}
	return actor, nil
}

// llmProxyEnv points Claude at Obot's LLM proxy when the session's environment enables it.
// Claude's self-hosted runner sets ANTHROPIC_BASE_URL for each session it starts, so the
// runner's command hook sets it from OBOT_ANTHROPIC_BASE_URL just before starting Claude.
func llmProxyEnv(runtime gt.ClaudeRuntime, obotURL string) []*pb.EnvVar {
	if !runtime.LLMProxy {
		return nil
	}
	return []*pb.EnvVar{
		{Name: "OBOT_ANTHROPIC_BASE_URL", Value: strings.TrimSuffix(obotURL, "/") + "/api/llm-proxy"},
		{Name: "ANTHROPIC_CUSTOM_HEADERS", Value: "X-Obot-Api-Key: " + llmKeyPlaceholder},
	}
}
