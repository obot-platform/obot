export interface MCPCatalogManifest {
	displayName: string;
	sourceURLs: string[];
	allowedUserIDs: string[];
}

export interface MCPCatalog extends MCPCatalogManifest {
	id: string;
}

export interface MCPCatalogSource {
	id: string;
}

export interface MCPCatalogEntry {
	id: string;
	manifest: MCPCatalogEntryManifest;
	readonly?: boolean;
	name?: string;
	deployments?: number;
	type: 'hosted' | 'remote';
}

export interface MCPCatalogEntryManifest {
	args?: string[];
	env?: {
		key: string;
		description: string;
	}[];
	command?: string;
	url?: string;
	headers?: {
		key: string;
		description: string;
	}[];
}

export interface OrgUser {
	created: string;
	username: string;
	email: string;
	explicitAdmin: boolean;
	role: number;
	iconURL: string;
	id: string;
}

export const Role = {
	ADMIN: 1
};
