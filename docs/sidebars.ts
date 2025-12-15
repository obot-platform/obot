// @ts-check

/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  sidebar: [
    "overview",
    "architecture",
    {
      type: "category",
      label: "Installation",
      items: [
        "installation/overview",
        "installation/docker-deployment",
        "installation/kubernetes-deployment",
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
      label: "Chat Interface",
      items: [
        "concepts/chat/overview",
        "concepts/chat/projects",
        "concepts/chat/threads",
        "concepts/chat/tasks",
      ],
    },
    {
      type: "category",
      label: "MCP Gateway",
      items: [
        "concepts/mcp-gateway/overview",
        "concepts/mcp-gateway/servers-and-tools",
        "concepts/mcp-gateway/obot-registry",
        "concepts/mcp-gateway/registry-api",
      ],
    },
    {
      type: "category",
      label: "Admin Interface",
      items: [
        "concepts/admin/overview",
        "concepts/admin/mcp-servers",
        "concepts/admin/mcp-server-catalogs",
        "concepts/admin/access-control",
        "concepts/admin/filters",
      ],
    },
    {
      type: "category",
      label: "Configuration",
      items: [
        "configuration/server-configuration",
        "configuration/auth-providers",
        "configuration/model-providers",
        "configuration/workspace-provider",
        {
          type: "category",
          label: "Advanced Configuration",
          items: [
            "configuration/oauth-configuration",
            "configuration/mcp-deployments-in-kubernetes",
            "configuration/audit-log-export",
          ],
        },
        {
          type: "category",
          label: "Encryption Providers",
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
    "integrations/ide-client-integration",
    "enterprise/overview",
    "faq",
  ],
};

export default sidebars;
