package mcp

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// SecurityPolicyProvider generates vendor-specific K8s objects for MCP server egress policies.
type SecurityPolicyProvider interface {
	Name() string
	Objects(server ServerConfig, mcpNamespace string) ([]kclient.Object, error)
	PruneTypes() []kclient.Object
}

var firewallPolicyGVK = schema.GroupVersionKind{
	Group:   "networking.aviatrix.com",
	Version: "v1alpha1",
	Kind:    "FirewallPolicy",
}

type aviatrixProvider struct{}

func (a *aviatrixProvider) Name() string { return "aviatrix" }

func (a *aviatrixProvider) PruneTypes() []kclient.Object {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(firewallPolicyGVK)
	return []kclient.Object{obj}
}

func (a *aviatrixProvider) Objects(server ServerConfig, mcpNamespace string) ([]kclient.Object, error) {
	if server.SecurityPolicy == nil {
		return nil, nil
	}
	policy := server.SecurityPolicy

	// Build inline SmartGroups
	smartGroups := []interface{}{
		// Source SmartGroup: selects MCP server pods by label
		map[string]interface{}{
			"name": server.MCPServerName + "-pods",
			"selectors": []interface{}{
				map[string]interface{}{
					"type":         "k8s",
					"k8sNamespace": mcpNamespace,
					"tags": map[string]interface{}{
						"app": server.MCPServerName,
					},
				},
			},
		},
		// Destination SmartGroup: any destination (for deny-all and domain-filtered rules)
		map[string]interface{}{
			"name": "any-destination",
			"selectors": []interface{}{
				map[string]interface{}{
					"cidr": "0.0.0.0/0",
				},
			},
		},
	}

	// Build inline WebGroups and rules from AllowedEgress
	var webGroups []interface{}
	var rules []interface{}

	for i, egress := range policy.AllowedEgress {
		ruleName := fmt.Sprintf("allow-%d", i)
		if egress.Description != "" {
			ruleName = fmt.Sprintf("allow-%s", sanitizeRuleName(egress.Description))
		}

		rule := map[string]interface{}{
			"name":   ruleName,
			"action": "permit",
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"app": server.MCPServerName,
				},
			},
			"destinationSmartGroups": []interface{}{
				map[string]interface{}{"name": "any-destination"},
			},
			"protocol": protocolOrDefault(egress.Protocol),
			"logging":  true,
		}

		// Add port if specified
		if len(egress.Ports) > 0 {
			rule["port"] = int64(egress.Ports[0])
			if len(egress.Ports) > 1 {
				rule["endPort"] = int64(egress.Ports[len(egress.Ports)-1])
			}
		}

		// Domain-based rules: create inline WebGroup and reference it in rule
		if len(egress.Domains) > 0 {
			wgName := fmt.Sprintf("%s-wg-%d", server.MCPServerName, i)
			webGroups = append(webGroups, map[string]interface{}{
				"name":    wgName,
				"domains": toStringInterfaceSlice(egress.Domains),
			})
			rule["webGroups"] = []interface{}{
				map[string]interface{}{"name": wgName},
			}
		}

		// CIDR-based rules: create additional destination SmartGroup
		if len(egress.CIDRs) > 0 {
			sgName := fmt.Sprintf("%s-dst-%d", server.MCPServerName, i)
			var selectors []interface{}
			for _, cidr := range egress.CIDRs {
				selectors = append(selectors, map[string]interface{}{
					"cidr": cidr,
				})
			}
			smartGroups = append(smartGroups, map[string]interface{}{
				"name":      sgName,
				"selectors": selectors,
			})
			rule["destinationSmartGroups"] = []interface{}{
				map[string]interface{}{"name": sgName},
			}
		}

		rules = append(rules, rule)
	}

	// Add deny-all rule last (rules are ordered, first match wins in Aviatrix DCF)
	if policy.DefaultAction == "" || policy.DefaultAction == "deny" {
		rules = append(rules, map[string]interface{}{
			"name":   "deny-all",
			"action": "deny",
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"app": server.MCPServerName,
				},
			},
			"destinationSmartGroups": []interface{}{
				map[string]interface{}{"name": "any-destination"},
			},
			"protocol": "any",
			"logging":  true,
		})
	}

	// Build the FirewallPolicy unstructured object
	spec := map[string]interface{}{
		"smartGroups": smartGroups,
		"rules":       rules,
	}
	if len(webGroups) > 0 {
		spec["webGroups"] = webGroups
	}

	fp := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.aviatrix.com/v1alpha1",
			"kind":       "FirewallPolicy",
			"metadata": map[string]interface{}{
				"name":      server.MCPServerName + "-fw",
				"namespace": mcpNamespace,
				"labels": map[string]interface{}{
					"app":                          server.MCPServerName,
					"app.kubernetes.io/managed-by": "obot",
					"obot.ai/security-provider":    "aviatrix",
				},
			},
			"spec": spec,
		},
	}

	return []kclient.Object{fp}, nil
}

func sanitizeRuleName(s string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(s) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			b.WriteRune(c)
		} else if c == ' ' || c == '_' {
			b.WriteRune('-')
		}
	}
	result := b.String()
	if len(result) > 40 {
		result = result[:40]
	}
	return result
}

func protocolOrDefault(p string) string {
	if p == "" {
		return "any"
	}
	return strings.ToLower(p)
}

func toStringInterfaceSlice(ss []string) []interface{} {
	result := make([]interface{}, len(ss))
	for i, s := range ss {
		result[i] = s
	}
	return result
}
