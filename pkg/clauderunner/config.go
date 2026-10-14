package clauderunner

import (
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation"
)

func DefaultRuntime(runtime *types.ClaudeRuntime, workerImage, runnerImage string) {
	if runtime.Image == "" {
		runtime.Image = runnerImage
	}
	if runtime.WorkerImage == "" {
		runtime.WorkerImage = workerImage
	}

	w := &runtime.Worker
	if w.WorkerReplicas == 0 {
		w.WorkerReplicas = 1
	}
	if w.CPULimit == "" {
		w.CPULimit = "2"
	}
	if w.MemoryLimit == "" {
		w.MemoryLimit = "4Gi"
	}

	r := &runtime.Resources
	if r.ActorCPU == "" {
		r.ActorCPU = "1"
	}
	if r.ActorMemory == "" {
		r.ActorMemory = "2Gi"
	}
	if r.IdleTimeoutMinutes == 0 {
		r.IdleTimeoutMinutes = 5
	}
	if r.SessionRetentionDays == 0 {
		r.SessionRetentionDays = 30
	}
}

func ValidateRuntime(runtime types.ClaudeRuntime) error {
	// The policy is delivered as a single environment variable to the runner.
	if len(runtime.ManagedSettings) > 64*1024 {
		return fmt.Errorf("managed settings must not exceed 64 KiB")
	}
	if len(runtime.ManagedSettings) > 0 {
		var settings map[string]json.RawMessage
		if err := json.Unmarshal(runtime.ManagedSettings, &settings); err != nil || settings == nil {
			return fmt.Errorf("managed settings must be a JSON object")
		}
	}

	if len(runtime.Image) > 512 || !regexp.MustCompile(`^[^@\s]+@sha256:[a-f0-9]{64}$`).MatchString(runtime.Image) {
		return fmt.Errorf("runner image must be pinned by SHA-256 digest (repository@sha256: followed by 64 lowercase hexadecimal characters)")
	}
	if runtime.WorkerImage == "" || len(runtime.WorkerImage) > 512 || strings.ContainsAny(runtime.WorkerImage, " \t\r\n") {
		return fmt.Errorf("a valid worker image is required")
	}
	if runtime.Worker.WorkerReplicas < 1 || runtime.Worker.WorkerReplicas > 100 {
		return fmt.Errorf("worker replicas must be between 1 and 100")
	}
	if err := ValidateEgressHosts(runtime.EgressHosts); err != nil {
		return err
	}

	return ValidateResources(runtime.Worker, runtime.Resources)
}

func ValidateEgressHosts(hosts []string) error {
	if len(hosts) > 100 {
		return fmt.Errorf("at most 100 egress hosts are allowed")
	}
	for _, host := range hosts {
		name := strings.TrimPrefix(host, "*.")
		if len(host) > 253 || net.ParseIP(name) != nil || len(validation.IsDNS1123Subdomain(name)) != 0 {
			return fmt.Errorf("egress hosts must be lowercase DNS hostnames, optionally prefixed with a wildcard (*.)")
		}
		for label := range strings.SplitSeq(name, ".") {
			if len(validation.IsDNS1123Label(label)) != 0 {
				return fmt.Errorf("egress hostname labels must contain 1–63 lowercase letters, digits, or hyphens and start and end with a letter or digit")
			}
		}
	}
	return nil
}

func ValidateResources(worker types.ClaudeWorker, r types.ClaudeResources) error {
	if r.IdleTimeoutMinutes < 1 || r.IdleTimeoutMinutes > 10080 {
		return fmt.Errorf("idle timeout must be between 1 and 10080 minutes")
	}
	if r.SessionRetentionDays != -1 && (r.SessionRetentionDays < 1 || r.SessionRetentionDays > 36500) {
		return fmt.Errorf("retention must be 1–36500 days or -1 to disable cleanup")
	}
	for _, value := range []string{worker.CPULimit, worker.MemoryLimit, r.ActorCPU, r.ActorMemory} {
		quantity, err := resource.ParseQuantity(value)
		if err != nil || quantity.Sign() <= 0 {
			return fmt.Errorf("resource quantities must be positive Kubernetes quantities")
		}
	}
	return nil
}
