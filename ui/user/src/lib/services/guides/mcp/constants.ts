import type { GuideHighlight, GuideListener } from '../types';

export const SIDEBAR_MCP_CATALOG_LINK = 'sidebar-link-mcp-catalog';
export const SIDEBAR_MCP_ACCESS_POLICIES_LINK = 'sidebar-link-mcp-access-policies';

export const highlightMcpCatalogLink: GuideHighlight = {
	selector: {
		id: SIDEBAR_MCP_CATALOG_LINK
	},
	title: 'MCP Catalog',
	description: 'Click here to view MCP server catalog.'
};

export const listenMcpCatalogLink: GuideListener = {
	id: SIDEBAR_MCP_CATALOG_LINK,
	action: {
		success: true
	}
};

export const highlightMcpAccessPoliciesLink: GuideHighlight = {
	selector: {
		id: SIDEBAR_MCP_ACCESS_POLICIES_LINK
	},
	title: 'MCP Access Policies',
	description: 'Click here to view MCP access policies.'
};

export const listenMcpAccessPoliciesLink: GuideListener = {
	id: SIDEBAR_MCP_ACCESS_POLICIES_LINK,
	action: {
		success: true
	}
};

export const addCatalogEntryDescriptions = {
	hosted:
		'A hosted MCP catalog entry allows you to add a custom MCP server that is managed and hosted by Obot. By having Obot host your MCP server, you can take advantage of automatic scaling, lifecycle management, API key management, and access policy enforcement. Once deployed in Obot, the server is available as a remote MCP URL that your MCP clients can consume.',
	remote:
		"A remote catalog entry allows you to proxy any remote MCP server through Obot, enabling you to take advantage of Obot's access policies, audit logging, and static OAuth integration.",
	composite:
		'A composite catalog entry serves two purposes. First, it allows you to combine multiple MCP servers and publish them as a single remote MCP server. Second, it enables you to expose only the tools you want users to access, giving you fine-grained control over which capabilities are published.'
};

export const obotCatalogEntryDescriptions = {
	hosted:
		'A hosted catalog entry provides a simple way to deploy and host an MCP server on the Obot platform, where Obot manages its operation and lifecycle. Click here to get started.',
	remote:
		"A remote catalog entry lets you proxy all traffic to a remote MCP server through Obot, enabling you to take advantage of Obot's access policies and audit logging. Click here to get started.",
	composite:
		'A composite catalog entry lets you combine multiple MCP servers into a single remote MCP server and expose only the tools you want users to access.'
};
