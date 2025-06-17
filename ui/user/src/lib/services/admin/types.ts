import type { MCPInfo } from '../chat/types';

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
	created: string;
	commandManifest?: MCPInfo;
	urlManifest?: MCPInfo;
	sourceURL?: string;
	type: string;
}

export interface MCPCatalogEntryManifest {
	server: MCPCatalogEntryServerManifest;
	url?: string;
	githubStars?: number;
	displayName?: string;
	metadata?: {
		categories?: string;
	};
	icon?: string;
}

export interface MCPCatalogEntryFieldManifest {
	key: string;
	description: string;
	name: string;
	required: boolean;
	sensitive: boolean;
	value: string;
}

export interface MCPCatalogEntryServerManifest {
	args?: string[];
	env?: MCPCatalogEntryFieldManifest[];
	command?: string;
	url?: string;
	headers?: MCPCatalogEntryFieldManifest[];
	name?: string;
}

export type MCPCatalogEntryFormData = Omit<MCPCatalogEntryManifest, 'metadata'> & {
	categories: string[];
};

export interface MCPCatalogServerManifest extends MCPCatalogEntryServerManifest {
	description?: string;
	icon?: string;
	catalogEntryID?: string;
	metadata?: {
		categories?: string;
	};
}

export interface OrgUser {
	created: string;
	username: string;
	email: string;
	explicitAdmin: boolean;
	role: number;
	iconURL: string;
	id: string;
	lastActiveDay?: string;
}

export const Role = {
	ADMIN: 1,
	USER: 10
};

export interface ProviderParameter {
	name: string;
	friendlyName?: string;
	description?: string;
	sensitive?: boolean;
	hidden?: boolean;
}

export interface BaseProvider {
	name: string;
	configured: boolean;
	created: string;
	missingConfigurationParameters?: string[];
	optionalConfigurationParameters?: ProviderParameter[];
	requiredConfigurationParameters?: ProviderParameter[];
	icon?: string;
	iconDark?: string;
	id: string;
	link?: string;
	namespace?: string;
	toolReference?: string;
}

export interface AuthProvider extends BaseProvider {
	type: 'authprovider';
}

export interface FileScannerProvider extends BaseProvider {
	type: 'filescannerprovider';
}

export interface FileScannerConfig {
	id: string;
	providerName: string;
	providerNamespace: string;
	updatedAt: string;
}

export interface Model {
	active: boolean;
	aliasAssigned: boolean;
	created: string;
	id: string;
	modelProvider: string;
	modelProviderName: string;
	name: string;
	targetModel: string;
	usage: string;
}

interface BaseThread {
	created: string;
	id: string;
	name: string;
	currentRunId?: string;
	projectID?: string;
	lastRunID?: string;
	userID?: string;
	project?: boolean;
	deleted?: string;
	systemTask?: boolean;
	ready?: boolean;
}

export type ProjectThread = BaseThread &
	({ assistantID: string; taskID?: never } | { assistantID?: never; taskID: string });
