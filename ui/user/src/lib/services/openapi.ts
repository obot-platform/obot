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

export interface OpenAPIImportDraft {
	config: OpenAPIRuntimeConfig;
	result: OpenAPIImportResult;
}

export async function importOpenAPI(config: OpenAPIRuntimeConfig): Promise<OpenAPIImportResult> {
	return (await doPost(
		'/openapi-import',
		{ source: config.source },
		{ dontLogErrors: true }
	)) as OpenAPIImportResult;
}
