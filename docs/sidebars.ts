// @ts-check

/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  sidebar: [
    "overview",
    {
      type: "category",
      label: "Concepts",
      items: [
        "concepts/projects",
        "concepts/threads",
        "concepts/tasks",
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
            {
              type: "category",
              label: "Encryption Providers",
              items: [
                "configuration/encryption-providers/aws-kms",
                "configuration/encryption-providers/azure-key-vault",
                "configuration/encryption-providers/google-cloud-kms"
              ]
            }
          ],
        },
        "enterprise"
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
