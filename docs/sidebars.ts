// @ts-check

/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  sidebar: [
    "overview",
    "architecture",
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
      label: "Installation",
      items: [
        "installation/general",
        {
          type: "category",
          label: "Configuration",
          items: [
            "configuration/server-configuration",
            "configuration/chat-configuration",
            "configuration/auth-providers",
            "configuration/workspace-provider",
            "configuration/model-providers",
            "configuration/oauth-configuration",
            {
              type: "category",
              label: "Reference Architectures",
              items: ["configuration/reference-architectures/gcp-gke"],
            },
            {
              type: "category",
              label: "Encryption Providers",
              items: [
                "configuration/encryption-providers/aws-kms",
                "configuration/encryption-providers/azure-key-vault",
                "configuration/encryption-providers/google-cloud-kms",
              ],
            },
          ],
        },
        "enterprise",
      ],
    },
    {
      type: "category",
      label: "Tutorials",
      items: [
        "tutorials/github-assistant",
        "tutorials/knowledge-assistant",
        "tutorials/slack-alerts-assistant",
      ],
    },
  ],
};

export default sidebars;
