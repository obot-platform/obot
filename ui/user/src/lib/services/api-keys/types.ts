export interface APIKey {
	id: number;
	userId: number;
	name: string;
	description?: string;
	canAccessSkills: boolean;
	createdAt: string;
	lastUsedAt?: string;
	expiresAt?: string;
	mcpServerIds?: string[];
}

export interface APIKeyCreateRequest {
	name: string;
	description?: string;
	expiresAt?: string;
	mcpServerIds: string[];
	canAccessSkills?: boolean;
}

export interface APIKeyCreateResponse extends APIKey {
	key: string; // Only shown once on creation
}
