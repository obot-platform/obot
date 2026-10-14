import type { MCPConfig, OpenAPIRuntimeConfig } from '.';
import { doPost } from './http';

export type OpenAPIMetadata = Partial<
	Record<'name' | 'description' | 'shortDescription' | 'icon', string>
>;

export interface OpenAPIDestinationValidation {
	valid: boolean;
	error?: string;
}

export interface OpenAPIImportResult {
	baseURL: string;
	destinationValidation: OpenAPIDestinationValidation;
	suggestedHeaders: MCPConfig[];
	suggestedMetadata: OpenAPIMetadata;
}

interface OpenAPIImportResponse extends Omit<OpenAPIImportResult, 'suggestedHeaders'> {
	suggestedHeaders: MCPConfig[] | null;
}

export interface OpenAPIImportDraft {
	config: OpenAPIRuntimeConfig;
	result: OpenAPIImportResult;
}

export async function importOpenAPI(config: OpenAPIRuntimeConfig): Promise<OpenAPIImportResult> {
	const result = (await doPost(
		'/openapi-import',
		{ source: config.source },
		{ dontLogErrors: true }
	)) as OpenAPIImportResponse;
	return { ...result, suggestedHeaders: result.suggestedHeaders ?? [] };
}

export async function validateOpenAPIDestination(
	config: OpenAPIRuntimeConfig,
	signal?: AbortSignal
): Promise<OpenAPIDestinationValidation> {
	const result = (await doPost(
		'/openapi-import',
		{ source: config.source, baseURL: config.baseURL },
		{ dontLogErrors: true, ...(signal ? { signal } : {}) }
	)) as OpenAPIImportResponse;
	return result.destinationValidation;
}
