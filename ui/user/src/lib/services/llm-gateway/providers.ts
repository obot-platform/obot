// Catalog of the upstream providers the embedded Bifrost gateway supports, with the
// configuration each one needs. Used by the provider setup forms.
import { m } from '$lib/i18n';

export interface KeyField {
	// Path of the value in the key payload, e.g. "value" or "azure_key_config.endpoint".
	path: string;
	label: string;
	placeholder?: string;
	description?: string;
	sensitive?: boolean;
	required?: boolean;
	multiline?: boolean;
}

export interface ProviderCatalogEntry {
	id: string;
	label: string;
	icon?: string;
	// A black logo, inverted in dark mode so it stays visible.
	monochromeIcon?: boolean;
	description: string;
	keyFields: KeyField[];
	// Whether a base URL is required (self-hosted providers).
	baseUrlRequired?: boolean;
	baseUrlPlaceholder?: string;
	// Providers that are used without any key.
	keyless?: boolean;
}

const apiKey = (placeholder = 'sk-...'): KeyField => ({
	path: 'value',
	label: m.models_providers_api_key(),
	placeholder,
	sensitive: true,
	required: true
});

export const PROVIDER_CATALOG: ProviderCatalogEntry[] = [
	{
		id: 'openai',
		label: 'OpenAI',
		icon: '/admin/assets/openai_icon.svg',
		monochromeIcon: true,
		description: m.models_providers_desc_openai(),
		keyFields: [apiKey()]
	},
	{
		id: 'anthropic',
		label: 'Anthropic',
		icon: '/admin/assets/anthropic_icon.svg',
		monochromeIcon: true,
		description: m.models_providers_desc_anthropic(),
		keyFields: [apiKey('sk-ant-...')]
	},
	{
		id: 'gemini',
		label: 'Google Gemini',
		icon: '/admin/assets/gemini_icon.svg',
		description: m.models_providers_desc_gemini(),
		keyFields: [apiKey('AIza...')]
	},
	{
		id: 'vertex',
		label: 'Google Vertex AI',
		icon: '/admin/assets/google_icon_small.png',
		description: m.models_providers_desc_vertex(),
		keyFields: [
			{
				path: 'vertex_key_config.project_id',
				label: m.models_providers_field_project_id(),
				required: true
			},
			{
				path: 'vertex_key_config.region',
				label: m.models_providers_field_region(),
				placeholder: 'us-central1',
				required: true
			},
			{
				path: 'vertex_key_config.auth_credentials',
				label: m.models_providers_field_service_account_json(),
				sensitive: true,
				multiline: true,
				description: m.models_providers_field_vertex_credentials_help()
			}
		]
	},
	{
		id: 'bedrock',
		label: 'AWS Bedrock',
		icon: '/admin/assets/amazon-bedrock.png',
		description: m.models_providers_desc_bedrock(),
		keyFields: [
			{
				path: 'value',
				label: m.models_providers_field_bedrock_api_key(),
				sensitive: true,
				description: m.models_providers_field_bedrock_api_key_help()
			},
			{ path: 'bedrock_key_config.access_key', label: m.models_providers_field_access_key_id() },
			{
				path: 'bedrock_key_config.secret_key',
				label: m.models_providers_field_secret_access_key(),
				sensitive: true
			},
			{
				path: 'bedrock_key_config.session_token',
				label: m.models_providers_field_session_token(),
				sensitive: true
			},
			{
				path: 'bedrock_key_config.region',
				label: m.models_providers_field_region(),
				placeholder: 'us-east-1',
				required: true
			},
			{
				path: 'bedrock_key_config.role_arn',
				label: m.models_providers_field_role_arn(),
				description: m.models_providers_field_role_arn_help()
			}
		]
	},
	{
		id: 'azure',
		label: 'Azure OpenAI / AI Foundry',
		icon: '/admin/assets/azure_openai_icon.svg',
		description: m.models_providers_desc_azure(),
		keyFields: [
			{
				path: 'azure_key_config.endpoint',
				label: m.models_providers_field_endpoint(),
				placeholder: 'https://my-resource.openai.azure.com',
				required: true
			},
			{
				path: 'value',
				label: m.models_providers_api_key(),
				sensitive: true,
				description: m.models_providers_field_azure_api_key_help()
			},
			{ path: 'azure_key_config.client_id', label: m.models_providers_field_entra_client_id() },
			{
				path: 'azure_key_config.client_secret',
				label: m.models_providers_field_entra_client_secret(),
				sensitive: true
			},
			{ path: 'azure_key_config.tenant_id', label: m.models_providers_field_entra_tenant_id() }
		]
	},
	{
		id: 'mistral',
		label: 'Mistral AI',
		description: m.models_providers_desc_mistral(),
		keyFields: [apiKey('')]
	},
	{
		id: 'groq',
		label: 'Groq',
		icon: '/admin/assets/groq_logo.svg',
		monochromeIcon: true,
		description: m.models_providers_desc_groq(),
		keyFields: [apiKey('gsk_...')]
	},
	{
		id: 'xai',
		label: 'xAI',
		icon: '/admin/assets/xai-logo.svg',
		monochromeIcon: true,
		description: m.models_providers_desc_xai(),
		keyFields: [apiKey('xai-...')]
	},
	{
		id: 'deepseek',
		label: 'DeepSeek',
		icon: '/admin/assets/deepseek-logo.svg',
		description: m.models_providers_desc_deepseek(),
		keyFields: [apiKey()]
	},
	{
		id: 'openrouter',
		label: 'OpenRouter',
		description: m.models_providers_desc_openrouter(),
		keyFields: [apiKey('sk-or-...')]
	},
	{
		id: 'cohere',
		label: 'Cohere',
		description: m.models_providers_desc_cohere(),
		keyFields: [apiKey('')]
	},
	{
		id: 'perplexity',
		label: 'Perplexity',
		description: m.models_providers_desc_perplexity(),
		keyFields: [apiKey('pplx-...')]
	},
	{
		id: 'cerebras',
		label: 'Cerebras',
		description: m.models_providers_desc_cerebras(),
		keyFields: [apiKey('csk-...')]
	},
	{
		id: 'fireworks',
		label: 'Fireworks AI',
		description: m.models_providers_desc_fireworks(),
		keyFields: [apiKey('fw_...')]
	},
	{
		id: 'huggingface',
		label: 'Hugging Face',
		description: m.models_providers_desc_huggingface(),
		keyFields: [apiKey('hf_...')]
	},
	{
		id: 'nebius',
		label: 'Nebius Token Factory',
		description: m.models_providers_desc_nebius(),
		keyFields: [apiKey('')]
	},
	{
		id: 'parasail',
		label: 'Parasail',
		description: m.models_providers_desc_parasail(),
		keyFields: [apiKey('')]
	},
	{
		id: 'databricks',
		label: 'Databricks',
		description: m.models_providers_desc_databricks(),
		keyFields: [
			{
				path: 'databricks_key_config.workspace_url',
				label: m.models_providers_field_workspace_url(),
				placeholder: 'https://my-workspace.cloud.databricks.com',
				required: true
			},
			{ path: 'value', label: m.models_providers_field_personal_access_token(), sensitive: true }
		]
	},
	{
		id: 'ollama',
		label: 'Ollama',
		description: m.models_providers_desc_ollama(),
		keyFields: [
			{
				path: 'ollama_key_config.url',
				label: m.models_providers_field_server_url(),
				placeholder: 'http://ollama:11434',
				required: true
			}
		]
	},
	{
		id: 'vllm',
		label: 'vLLM',
		icon: '/admin/assets/vllm-logo.svg',
		description: m.models_providers_desc_vllm(),
		keyFields: [
			{
				path: 'vllm_key_config.url',
				label: m.models_providers_field_server_url(),
				placeholder: 'http://vllm:8000',
				required: true
			},
			{
				path: 'vllm_key_config.model_name',
				label: m.models_providers_field_served_model_name(),
				required: true
			},
			{ path: 'value', label: m.models_providers_api_key(), sensitive: true }
		]
	},
	{
		id: 'sgl',
		label: 'SGLang',
		description: m.models_providers_desc_sglang(),
		keyFields: [
			{
				path: 'sgl_key_config.url',
				label: m.models_providers_field_server_url(),
				placeholder: 'http://sglang:30000'
			}
		]
	},
	{
		id: 'replicate',
		label: 'Replicate',
		description: m.models_providers_desc_replicate(),
		keyFields: [apiKey('r8_...')]
	},
	{
		id: 'elevenlabs',
		label: 'ElevenLabs',
		description: m.models_providers_desc_elevenlabs(),
		keyFields: [apiKey('')]
	}
];

// Custom providers speak one of these APIs at their own base URL.
export const CUSTOM_PROVIDER_BASES = [
	{ id: 'openai', label: m.models_providers_compatible({ name: 'OpenAI' }) },
	{ id: 'anthropic', label: m.models_providers_compatible({ name: 'Anthropic' }) },
	{ id: 'gemini', label: m.models_providers_compatible({ name: 'Gemini' }) },
	{ id: 'cohere', label: m.models_providers_compatible({ name: 'Cohere' }) }
];

// The request types of an OpenAI-compatible server that serves only Chat Completions (and
// model listing and embeddings). Without the Responses API, the gateway serves Responses and
// Anthropic Messages requests to it through Chat Completions.
export const CHAT_COMPLETIONS_ONLY_REQUESTS: Record<string, boolean> = {
	list_models: true,
	model_retrieve: true,
	text_completion: true,
	text_completion_stream: true,
	chat_completion: true,
	chat_completion_stream: true,
	embedding: true
};

export function catalogEntry(id: string): ProviderCatalogEntry | undefined {
	return PROVIDER_CATALOG.find((p) => p.id === id);
}

export function providerLabel(id: string): string {
	return catalogEntry(id)?.label ?? id;
}

export function providerIcon(id: string): string | undefined {
	return catalogEntry(id)?.icon;
}

// Classes for a provider's logo: black logos are inverted in dark mode.
export function providerIconClass(id: string): string {
	return catalogEntry(id)?.monochromeIcon ? 'dark:invert' : '';
}

export function getPath(obj: Record<string, unknown>, path: string): unknown {
	return path.split('.').reduce<unknown>((acc, part) => {
		if (acc && typeof acc === 'object') return (acc as Record<string, unknown>)[part];
		return undefined;
	}, obj);
}

export function setPath(obj: Record<string, unknown>, path: string, value: unknown) {
	const parts = path.split('.');
	let cur = obj;
	for (const part of parts.slice(0, -1)) {
		if (!cur[part] || typeof cur[part] !== 'object') cur[part] = {};
		cur = cur[part] as Record<string, unknown>;
	}
	cur[parts[parts.length - 1]] = value;
}
