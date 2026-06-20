package mcp

import (
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type ResourceMaximums struct {
	CPURequest    *resource.Quantity
	CPULimit      *resource.Quantity
	MemoryRequest *resource.Quantity
	MemoryLimit   *resource.Quantity
}

type ResourceMaximumExceededError struct {
	Field   string
	Actual  resource.Quantity
	Maximum resource.Quantity
}

func (e *ResourceMaximumExceededError) Error() string {
	return fmt.Sprintf("%s %s exceeds configured maximum %s", e.Field, e.Actual.String(), e.Maximum.String())
}

func ParseResourceMaximums(opts Options) (ResourceMaximums, error) {
	cpuRequest, err := parseResourceMaximum("MCPK8sMaxCPURequest", opts.MCPK8sMaxCPURequest)
	if err != nil {
		return ResourceMaximums{}, err
	}
	cpuLimit, err := parseResourceMaximum("MCPK8sMaxCPULimit", opts.MCPK8sMaxCPULimit)
	if err != nil {
		return ResourceMaximums{}, err
	}
	memoryRequest, err := parseResourceMaximum("MCPK8sMaxMemoryRequest", opts.MCPK8sMaxMemoryRequest)
	if err != nil {
		return ResourceMaximums{}, err
	}
	memoryLimit, err := parseResourceMaximum("MCPK8sMaxMemoryLimit", opts.MCPK8sMaxMemoryLimit)
	if err != nil {
		return ResourceMaximums{}, err
	}

	return ResourceMaximums{
		CPURequest:    cpuRequest,
		CPULimit:      cpuLimit,
		MemoryRequest: memoryRequest,
		MemoryLimit:   memoryLimit,
	}, nil
}

func parseResourceMaximum(field, value string) (*resource.Quantity, error) {
	if value == "" {
		return nil, nil
	}
	quantity, err := resource.ParseQuantity(value)
	if err != nil {
		return nil, fmt.Errorf("invalid %s %q: %w", field, value, err)
	}
	if quantity.Sign() < 0 {
		return nil, fmt.Errorf("invalid %s %q: must be non-negative", field, value)
	}
	return &quantity, nil
}

func (m ResourceMaximums) Empty() bool {
	return m.CPURequest == nil &&
		m.CPULimit == nil &&
		m.MemoryRequest == nil &&
		m.MemoryLimit == nil
}

func (m ResourceMaximums) Validate(resources corev1.ResourceRequirements) error {
	if m.Empty() {
		return nil
	}
	if err := validateResourceMaximum("resources.requests.cpu", resources.Requests, corev1.ResourceCPU, m.CPURequest); err != nil {
		return err
	}
	if err := validateResourceMaximum("resources.limits.cpu", resources.Limits, corev1.ResourceCPU, m.CPULimit); err != nil {
		return err
	}
	if err := validateResourceMaximum("resources.requests.memory", resources.Requests, corev1.ResourceMemory, m.MemoryRequest); err != nil {
		return err
	}
	return validateResourceMaximum("resources.limits.memory", resources.Limits, corev1.ResourceMemory, m.MemoryLimit)
}

func validateResourceMaximum(field string, resources corev1.ResourceList, resourceName corev1.ResourceName, maximum *resource.Quantity) error {
	if maximum == nil {
		return nil
	}
	actual, ok := resources[resourceName]
	if !ok {
		return nil
	}
	if actual.Cmp(*maximum) <= 0 {
		return nil
	}
	return &ResourceMaximumExceededError{
		Field:   field,
		Actual:  actual,
		Maximum: *maximum,
	}
}

func ValidateK8sSettingsResourceMaximums(k8sSettings v1.K8sSettingsSpec, maximums ResourceMaximums) error {
	// We are relying on the mcpContainerResources function just to translate the k8ssettings type to corev1.ResourceRequirements.
	if err := maximums.Validate(mcpContainerResources(nil, types.RuntimeNPX, false, k8sSettings)); err != nil {
		return fmt.Errorf("default MCP server resources exceed maximums: %w", err)
	}
	if err := maximums.Validate(mcpContainerResources(nil, types.RuntimeNPX, true, k8sSettings)); err != nil {
		return fmt.Errorf("default nanobot agent MCP server resources exceed maximums: %w", err)
	}
	return nil
}

func ValidateConfiguredK8sSettingsResourceMaximums(k8sSettings v1.K8sSettingsSpec, maximums ResourceMaximums) error {
	if maximums.Empty() {
		return nil
	}
	if k8sSettings.Resources != nil {
		if err := maximums.Validate(mcpContainerResources(nil, types.RuntimeNPX, false, k8sSettings)); err != nil {
			return fmt.Errorf("configured default MCP server resources exceed maximums: %w", err)
		}
	}
	if k8sSettings.NanobotAgentResources != nil {
		if err := maximums.Validate(mcpContainerResources(nil, types.RuntimeNPX, true, k8sSettings)); err != nil {
			return fmt.Errorf("configured default nanobot agent MCP server resources exceed maximums: %w", err)
		}
	}
	return nil
}
