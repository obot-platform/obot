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
	editable?: boolean;
}

export interface MCPCatalogEntryManifest {
	server: MCPCatalogEntryServerManifest;
	url?: string;
	githubStars?: number;
	metadata?: Record<string, string>;
}

export interface MCPCatalogEntryFieldManifest {
	key: string;
	description: string;
	name: string;
	required?: boolean;
	sensitive?: boolean;
}

export interface MCPCatalogEntryServerManifest {
	args?: string[];
	env?: MCPCatalogEntryFieldManifest[];
	command?: string;
	url?: string;
	headers?: MCPCatalogEntryFieldManifest[];
	name?: string;
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

export interface AuthProvider {
	configured: boolean;
	created: string;
	icon: string;
	iconDark?: string;
	id: string;
	link: string;
	name: string;
	namespace: string;
	missingConfigurationParameters?: string[];
	optionalConfigurationParameters: string[];
	requiredConfigurationParameters: string[];
	toolReference: string;
}

export interface FileScannerProvider {
	configured: boolean;
	created: string;
	id: string;
	icon: string;
	iconDark?: string;
	link: string;
	missingConfigurationParameters?: string[];
	optionalConfigurationParameters: string[];
	requiredConfigurationParameters: string[];
	name: string;
	namespace: string;
	toolReference: string;
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
