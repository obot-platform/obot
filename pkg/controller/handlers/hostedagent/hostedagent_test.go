package hostedagent

import (
	"context"
	"strings"
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentbackend"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
)

func TestDefaultDesiredBuilder(t *testing.T) {
	instance := &v1.HostedAgentInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "instance",
			Namespace: "default",
			UID:       ktypes.UID("instance-uid"),
		},
		Spec: v1.HostedAgentInstanceSpec{
			UserID: "user-1",
			PoolID: "pool-1",
			Manifest: types2.HostedAgentInstanceManifest{
				Name:    "My agent",
				Answers: map[string]string{"public": "yes", "secret": "no"},
				Skills:  []string{"personal"},
			},
		},
	}
	agent := &v1.HostedAgent{
		Spec: v1.HostedAgentSpec{
			Manifest: types2.HostedAgentManifest{
				HarnessID:  "harness-1",
				GitRepo:    "https://example.com/repo.git",
				MCPServers: []string{"mcp-1"},
				Models:     []string{"model-1"},
				Skills:     []string{"shared"},
				Env: []types2.HostedAgentEnv{
					{Key: "PUBLIC", Value: "value"},
					{Key: "SECRET", Value: "must-not-leak", Sensitive: true},
				},
				Questions: []types2.HostedAgentQuestion{
					{Key: "public"},
					{Key: "secret", Sensitive: true},
				},
			},
		},
	}
	harness := &v1.Harness{
		Spec: v1.HarnessSpec{
			Manifest: types2.HarnessManifest{Image: "example/image:latest"},
		},
	}

	desired, err := (defaultDesiredBuilder{}).Build(context.Background(), BuildInput{
		Instance: instance, Agent: agent, Harness: harness,
	})
	if err != nil {
		t.Fatal(err)
	}

	if desired.Image != "example/image:latest" {
		t.Fatalf("unexpected image %q", desired.Image)
	}
	if desired.Source.URL != "https://example.com/repo.git" {
		t.Fatalf("unexpected source %q", desired.Source.URL)
	}
	if desired.Env["PUBLIC"] != "value" {
		t.Fatalf("public environment was not rendered: %#v", desired.Env)
	}
	if _, ok := desired.Env["SECRET"]; ok {
		t.Fatal("sensitive environment leaked into desired configuration")
	}
	// The config is an ordinary readable file; only the credentials that go
	// with it are a secret.
	config := ""
	for _, file := range desired.Files {
		if file.Path == agentConfigPath {
			config = string(file.Content)
		}
	}
	if config == "" {
		t.Fatalf("no agent config was delivered: %#v", desired.Files)
	}
	for _, expected := range []string{"mcp-1", `"public":"yes"`} {
		if !strings.Contains(config, expected) {
			t.Errorf("agent config %q does not contain %q", config, expected)
		}
	}
	for _, excluded := range []string{"must-not-leak", `"secret":"no"`} {
		if strings.Contains(config, excluded) {
			t.Errorf("agent config contains sensitive value %q", excluded)
		}
	}
	if !strings.HasPrefix(desired.Revision, "sha256:") {
		t.Fatalf("unexpected revision %q", desired.Revision)
	}
}

func TestApplyObservationRequiresCurrentRevisionAndKeepsURL(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	handler := &Handler{now: func() time.Time { return now }}
	instance := &v1.HostedAgentInstance{}

	handler.applyObservation(instance, "wanted", agentbackend.InstanceObservation{
		Exists:           true,
		State:            agentbackend.StateReady,
		ObservedRevision: "old",
		URL:              "https://agent.example",
		Ref:              agentbackend.InstanceRef{BackendID: "backend-1"},
	})
	if instance.Status.State != types2.HostedAgentStatePending {
		t.Fatalf("stale revision must remain pending, got %q", instance.Status.State)
	}

	handler.applyObservation(instance, "wanted", agentbackend.InstanceObservation{
		Exists:           true,
		State:            agentbackend.StateReady,
		ObservedRevision: "wanted",
	})
	if instance.Status.State != types2.HostedAgentStateReady {
		t.Fatalf("current ready revision was %q", instance.Status.State)
	}
	if instance.Status.URL != "https://agent.example" {
		t.Fatalf("URL was not sticky: %q", instance.Status.URL)
	}
	if instance.Status.BackendID != "backend-1" {
		t.Fatalf("backend ID was not retained: %q", instance.Status.BackendID)
	}
	if instance.Status.LastObservedTime == nil || !instance.Status.LastObservedTime.Time.Equal(now) {
		t.Fatalf("unexpected observation time %#v", instance.Status.LastObservedTime)
	}
}

func TestDesiredRevisionIgnoresBackendID(t *testing.T) {
	desired := agentbackend.DesiredInstance{
		Ref:   agentbackend.InstanceRef{ID: "instance", BackendID: "first"},
		Image: "image",
	}
	first, err := desiredRevision(desired)
	if err != nil {
		t.Fatal(err)
	}
	desired.Ref.BackendID = "second"
	second, err := desiredRevision(desired)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("backend ID changed revision: %q != %q", first, second)
	}
}

func TestPoolSelectionAndNames(t *testing.T) {
	assignments := []v1.HostedAgentPoolAssignment{
		{Spec: v1.HostedAgentPoolAssignmentSpec{
			Manifest: types2.HostedAgentPoolAssignmentManifest{
				UserID: "user-1",
				PoolID: "secondary",
			},
		}},
		{Spec: v1.HostedAgentPoolAssignmentSpec{
			Manifest: types2.HostedAgentPoolAssignmentManifest{
				UserID:  "user-1",
				PoolID:  "primary",
				Default: true,
			},
		}},
	}
	if got := defaultPool(assignments); got != "primary" {
		t.Fatalf("unexpected default pool %q", got)
	}
	if !isAssigned(assignments, "secondary") || isAssigned(assignments, "missing") {
		t.Fatal("assignment membership was evaluated incorrectly")
	}

	pool1, assignment1 := initialPoolNames("user-1")
	pool2, assignment2 := initialPoolNames("user-1")
	otherPool, _ := initialPoolNames("user-2")
	if pool1 != pool2 || assignment1 != assignment2 {
		t.Fatal("initial pool names are not deterministic")
	}
	if pool1 == otherPool {
		t.Fatal("different users received the same pool name")
	}
	if !strings.HasPrefix(pool1, "hp-") || !strings.HasPrefix(assignment1, "hps-") {
		t.Fatalf("unexpected names %q and %q", pool1, assignment1)
	}
}

// A status write emits a change event that reconciles the instance again at
// once. Rewriting an identical status therefore makes the controller drive its
// own next pass, which pegs a CPU rather than honouring the poll interval.
func TestStatusUnchangedIgnoresHeartbeatOnly(t *testing.T) {
	earlier := metav1.NewTime(time.Unix(1000, 0))
	later := metav1.NewTime(time.Unix(2000, 0))

	base := v1.HostedAgentInstanceStatus{
		State:            types2.HostedAgentStateReady,
		URL:              "http://sandbox",
		ObservedRevision: "sha256:abc",
		BackendID:        "obot-agent-1",
		LastObservedTime: &earlier,
	}
	same := base
	same.LastObservedTime = &later

	if !statusUnchanged(base, same) {
		t.Error("a moved heartbeat alone must not count as a status change")
	}

	changed := base
	changed.State = types2.HostedAgentStateError
	if statusUnchanged(base, changed) {
		t.Error("a state change must count as a status change")
	}
}

func TestHeartbeatDue(t *testing.T) {
	last := metav1.NewTime(time.Unix(1000, 0))

	if heartbeatDue(&last, time.Unix(1000, 0).Add(time.Second), time.Minute) {
		t.Error("the heartbeat is not due before the poll interval has elapsed")
	}
	if !heartbeatDue(&last, time.Unix(1000, 0).Add(time.Minute), time.Minute) {
		t.Error("the heartbeat is due once the poll interval has elapsed")
	}
	if !heartbeatDue(nil, time.Unix(1000, 0), time.Minute) {
		t.Error("an instance that has never been observed is always due")
	}
}
