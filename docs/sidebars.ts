// @ts-check

/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  sidebar: [
    "overview",
    {
      type: "category",
      label: "Concepts",
      items: [
        "concepts/mcp-hosting",
        "concepts/mcp-registry",
        "concepts/mcp-gateway",
        "concepts/obot-agent",
        "concepts/obot-chat",
        "concepts/architecture",
      ],
    },
    {
      type: "category",
      label: "Features",
      items: [
        "functionality/overview",
        "functionality/mcp-servers",
        "functionality/mcp-registries",
        "functionality/audit-logs-and-usage",
        "functionality/filters",
        "functionality/server-scheduling",
        "functionality/obot-agent-management",
        "functionality/chat-management",
        "functionality/model-access-policies",
        "functionality/message-policies",
        "functionality/skills",
        "functionality/skill-access-policies",
        "functionality/user-management",
        "functionality/api-keys",
        "functionality/branding",
        "functionality/agent/overview",
        "functionality/workflow-sharing",
        "functionality/chat/overview",
      ],
    },
    {
      type: "category",
      label: "Installation",
      items: [
        "installation/overview",
        "installation/docker-deployment",
        "installation/kubernetes-deployment",
        "installation/kubernetes-persistent-storage",
        "installation/enabling-authentication",
        {
          type: "category",
          label: "Reference Architectures",
          items: [
            "installation/reference-architectures/gcp-gke",
            "installation/reference-architectures/aws-eks",
            "installation/reference-architectures/azure-aks",
          ],
        },
      ],
    },
    {
      type: "category",
      label: "Configuration and Operations",
      items: [
        "configuration/auth-providers",
        "configuration/model-providers",
        "configuration/workspace-provider",
        "configuration/user-roles",
        "configuration/mcp-server-gitops",
        "configuration/mcp-deployments-in-kubernetes",
        "configuration/image-pull-secrets",
        "configuration/mcp-server-egress-control",
        "configuration/audit-log-export",
        "configuration/mcp-server-oauth-configuration",
        "configuration/server-configuration",
        {
          type: "category",
          label: "Encryption",
          items: [
            "configuration/encryption-providers/overview",
            "configuration/encryption-providers/aws-kms",
            "configuration/encryption-providers/azure-key-vault",
            "configuration/encryption-providers/google-cloud-kms",
            "configuration/encryption-providers/custom-provider",
          ],
        },
      ],
    },
    "enterprise/overview",
    "faq",
  ],
};

export default sidebars;
