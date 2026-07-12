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
				"concepts/architecture",
			],
		},
		{
			type: "category",
			label: "Features",
			items: [
				"functionality/overview",
				"functionality/mcp-servers",
				"functionality/mcp-access-policies",
				"functionality/mcp-registry-api",
				"functionality/audit-logs-and-usage",
				"functionality/filters",
				"functionality/server-scheduling",
				"functionality/obot-agent-management",
				"functionality/model-access-policies",
				"functionality/llm-gateway",
				"functionality/message-policies",
				"functionality/skills",
				"functionality/skill-access-policies",
				"functionality/device-management",
				"functionality/user-management",
				"functionality/api-keys",
				"functionality/branding",
				"functionality/workflow-sharing",
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
				"installation/cli-setup",
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
				{
					type: "category",
					label: "Tutorials",
					items: ["configuration/tutorials/slack-mcp-server"],
				},
			],
		},
		"enterprise/overview",
		"faq",
	],
};

export default sidebars;
