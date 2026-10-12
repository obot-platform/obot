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

export async function validateOpenAPIDestination(
	config: OpenAPIRuntimeConfig
): Promise<OpenAPIDestinationValidation> {
	const result = (await doPost(
		'/openapi-import',
		{ source: config.source, baseURL: config.baseURL },
		{ dontLogErrors: true }
	)) as OpenAPIImportResult;
	return result.destinationValidation;
}
