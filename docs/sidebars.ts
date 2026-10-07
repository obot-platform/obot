// @ts-check

/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
	"sidebar": [
		{
			"type": "category",
			"label": "Start Here",
			"link": {
				"type": "doc",
				"id": "start-here/connect"
			},
			"items": [
				{
					"type": "doc",
					"id": "overview",
					"label": "What is Obot?"
				},
				{
					"type": "doc",
					"id": "start-here/connect",
					"label": "Connect to a vMCP"
				},
				{
					"type": "doc",
					"id": "start-here/govern",
					"label": "Create a vMCP"
				},
				{
					"type": "doc",
					"id": "start-here/choose",
					"label": "Explore Obot"
				},
				{
					"type": "doc",
					"id": "enterprise/overview",
					"label": "Editions and feature availability"
				}
			]
		},
		{
			"type": "category",
			"label": "MCP Gateway",
			"link": {
				"type": "doc",
				"id": "concepts/mcp-gateway"
			},
			"items": [
				{
					"type": "doc",
					"id": "concepts/mcp-gateway",
					"label": "vMCP overview"
				},
				{
					"type": "doc",
					"id": "mcp-gateway/server-types",
					"label": "Create vMCP"
				},
				{
					"type": "doc",
					"id": "mcp-gateway/publish",
					"label": "Share vMCPs"
				},
				{
					"type": "doc",
					"id": "mcp-gateway/connect-clients",
					"label": "Connect AI clients"
				},
				{
					"type": "doc",
					"id": "mcp-gateway/access",
					"label": "Control MCP access"
				},
				{
					"type": "doc",
					"id": "mcp-gateway/register-remote",
					"label": "Add remote MCP servers"
				},
				{
					"type": "doc",
					"id": "concepts/mcp-hosting",
					"label": "Add hosted MCP servers"
				},
				{
					"type": "doc",
					"id": "functionality/filters",
					"label": "Filter MCP traffic"
				},
				{
					"type": "doc",
					"id": "functionality/mcp-tunnels",
					"label": "Connect private-network servers"
				},
				{
					"type": "doc",
					"id": "mcp-gateway/troubleshooting",
					"label": "Troubleshooting"
				}
			]
		},
		{
			"type": "category",
			"label": "LLM Gateway",
			"link": {
				"type": "doc",
				"id": "llm-gateway/how-it-works"
			},
			"items": [
				{
					"type": "doc",
					"id": "llm-gateway/how-it-works",
					"label": "Overview"
				},
				{
					"type": "doc",
					"id": "configuration/model-providers",
					"label": "Configure providers and models"
				},
				{
					"type": "doc",
					"id": "llm-gateway/connect-clients",
					"label": "Connect clients and applications"
				},
				{
					"type": "doc",
					"id": "functionality/model-access-policies",
					"label": "Control model access"
				},
				{
					"type": "doc",
					"id": "llm-gateway/audit",
					"label": "Audit usage and costs"
				},
				{
					"type": "doc",
					"id": "llm-gateway/compatibility",
					"label": "API compatibility"
				},
				{
					"type": "doc",
					"id": "llm-gateway/troubleshooting",
					"label": "Troubleshooting"
				}
			]
		},
		{
			"type": "category",
			"label": "Registries & Skills",
			"link": {
				"type": "doc",
				"id": "registries/overview"
			},
			"items": [
				{
					"type": "doc",
					"id": "registries/overview",
					"label": "Overview"
				},
				{
					"type": "doc",
					"id": "concepts/mcp-registry",
					"label": "MCP Servers"
				},
				{
					"type": "doc",
					"id": "registries/publish-skills",
					"label": "Skills"
				},
				{
					"type": "doc",
					"id": "configuration/mcp-server-gitops",
					"label": "Git Catalogs"
				}
			]
		},
		{
			"type": "category",
			"label": "Device Management (Sentry)",
			"link": {
				"type": "doc",
				"id": "device-management/how-sentry-works"
			},
			"items": [
				{
					"type": "doc",
					"id": "device-management/how-sentry-works",
					"label": "Overview"
				},
				{
					"type": "doc",
					"id": "device-management/enroll",
					"label": "Enroll devices"
				},
				{
					"type": "doc",
					"id": "device-management/inventory",
					"label": "Inspect device inventory"
				},
				{
					"type": "doc",
					"id": "device-management/enforcement",
					"label": "Policy enforcement"
				}
			]
		},
		{
			"type": "category",
			"label": "Security & Governance",
			"link": {
				"type": "doc",
				"id": "security/model"
			},
			"items": [
				{
					"type": "doc",
					"id": "security/model",
					"label": "Overview"
				},
				{
					"type": "doc",
					"id": "security/authentication",
					"label": "User sign-in and identity"
				},
				{
					"type": "doc",
					"id": "security/policy-coverage",
					"label": "Roles and access policies"
				},
				{
					"type": "doc",
					"id": "security/credentials",
					"label": "Encryption and secrets"
				},
				{
					"type": "doc",
					"id": "security/isolation",
					"label": "Isolate workloads"
				},
				{
					"type": "doc",
					"id": "security/audit-data",
					"label": "Audit data and retention"
				}
			]
		},
		{
			"type": "category",
			"label": "Deploy & Operate",
			"link": {
				"type": "doc",
				"id": "installation/overview"
			},
			"items": [
				{
					"type": "doc",
					"id": "installation/overview",
					"label": "Overview"
				},
				{
					"type": "doc",
					"id": "installation/docker-deployment",
					"label": "Docker (evaluation)"
				},
				{
					"type": "doc",
					"id": "installation/kubernetes-deployment",
					"label": "Kubernetes (production)"
				},
				{
					"type": "doc",
					"id": "configuration/auth-providers",
					"label": "Configure authentication providers"
				},
				{
					"type": "category",
					"label": "Cloud deployments",
					"collapsible": false,
					"collapsed": false,
					"items": [
						{
							"type": "doc",
							"id": "installation/reference-architectures/aws-eks",
							"label": "Amazon EKS"
						},
						{
							"type": "doc",
							"id": "installation/reference-architectures/azure-aks",
							"label": "Azure AKS"
						},
						{
							"type": "doc",
							"id": "installation/reference-architectures/gcp-gke",
							"label": "Google GKE"
						}
					]
				},
				{
					"type": "doc",
					"id": "functionality/branding",
					"label": "Customize branding"
				},
				{
					"type": "doc",
					"id": "operations/capacity",
					"label": "Capacity"
				},
				{
					"type": "doc",
					"id": "operations/high-availability",
					"label": "High availability"
				},
				{
					"type": "doc",
					"id": "operations/monitoring",
					"label": "Monitoring"
				},
				{
					"type": "doc",
					"id": "operations/troubleshooting",
					"label": "Troubleshooting"
				},
				{
					"type": "doc",
					"id": "operations/backup",
					"label": "Backup and recovery"
				},
				{
					"type": "doc",
					"id": "operations/upgrades",
					"label": "Upgrades and rollback"
				}
			]
		},
		{
			"type": "category",
			"label": "Architecture",
			"link": {
				"type": "doc",
				"id": "concepts/architecture"
			},
			"items": [
				{
					"type": "doc",
					"id": "concepts/architecture",
					"label": "Overview"
				},
				{
					"type": "doc",
					"id": "architecture/request-flows",
					"label": "Request flows and trust boundaries"
				},
				{
					"type": "doc",
					"id": "architecture/identity-resources",
					"label": "Resource ownership"
				},
				{
					"type": "doc",
					"id": "architecture/topologies",
					"label": "Where Obot and MCP servers run"
				},
				{
					"type": "doc",
					"id": "architecture/data-lifecycle",
					"label": "Data storage and lifecycle"
				}
			]
		},
		{
			"type": "category",
			"label": "Reference",
			"link": {
				"type": "generated-index",
				"title": "Reference",
				"slug": "/reference",
				"description": "Configuration, commands, APIs, schemas, and release information."
			},
			"items": [
				{
					"type": "doc",
					"id": "configuration/server-configuration",
					"label": "Server configuration"
				},
				{
					"type": "doc",
					"id": "reference/helm",
					"label": "Helm values"
				},
				{
					"type": "doc",
					"id": "reference/cli-api",
					"label": "CLI and APIs"
				},
				{
					"type": "doc",
					"id": "reference/catalog-schemas",
					"label": "Catalog schemas"
				},
				{
					"type": "doc",
					"id": "reference/filter-schemas",
					"label": "Filter schemas"
				},
				{
					"type": "doc",
					"id": "reference/compatibility",
					"label": "Release notes and compatibility"
				}
			]
		}
	]
};

export default sidebars;
