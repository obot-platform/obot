import type { MCPConfig, OpenAPIRuntimeConfig } from '.';
import { doPost } from './http';

export type OpenAPIMetadata = Partial<
	Record<'name' | 'description' | 'shortDescription' | 'icon', string>
>;

function object(value: unknown): Record<string, unknown> {
	return value && typeof value === 'object' && !Array.isArray(value)
		? (value as Record<string, unknown>)
		: {};
}

export function openAPIBaseURL(schema?: Record<string, unknown>): string {
	if (!Array.isArray(schema?.servers)) return '';
	for (const server of schema.servers) {
		const value = object(server).url;
		if (typeof value !== 'string' || /[{}]/.test(value)) continue;
		try {
			const url = new URL(value);
			if (
				['http:', 'https:'].includes(url.protocol) &&
				!url.username &&
				!url.password &&
				!url.search &&
				!url.hash
			)
				return value;
		} catch {
			/* Skip relative server URLs, matching the importer. */
		}
	}
	return '';
}

export interface OpenAPIImportResult {
	schema: Record<string, unknown>;
	baseURL: string;
	suggestedHeaders: MCPConfig[];
	suggestedMetadata: OpenAPIMetadata;
}

export interface OpenAPIImportDraft {
	config: OpenAPIRuntimeConfig;
	result: OpenAPIImportResult;
}

export async function importOpenAPI(
	entity: 'catalog' | 'workspace',
	id: string,
	config: OpenAPIRuntimeConfig
): Promise<OpenAPIImportResult> {
	const collection = entity === 'workspace' ? 'workspaces' : 'mcp-catalogs';
	return (await doPost(
		`/${collection}/${encodeURIComponent(id)}/openapi/import`,
		{ source: config.source, baseURL: config.baseURL },
		{ dontLogErrors: true }
	)) as OpenAPIImportResult;
}
