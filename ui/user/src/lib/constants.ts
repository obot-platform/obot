export const ABORTED_THREAD_MESSAGE = 'thread was aborted, cancelling run';
export const ABORTED_BY_USER_MESSAGE = 'aborted by user';

export const UNAUTHORIZED_PATHS = new Set(['/', '/privacy-policy', '/terms-of-service', '/admin']);

export const PAGE_TRANSITION_DURATION = 200;
export const PAGE_SIZE = 50;

export const CommonModelProviderIds = {
	OLLAMA: 'ollama-model-provider',
	GENERIC_RESPONSES: 'generic-responses-model-provider',
	GROQ: 'groq-model-provider',
	VLLM: 'vllm-model-provider',
	ANTHROPIC: 'anthropic-model-provider',
	OPENAI: 'openai-model-provider',
	AZURE_OPENAI: 'azure-openai-model-provider',
	AMAZON_BEDROCK: 'amazon-bedrock-model-provider',
	AMAZON_BEDROCK_API_KEY: 'amazon-bedrock-api-key-model-provider',
	ANTHROPIC_BEDROCK: 'anthropic-bedrock-model-provider',
	XAI: 'xai-model-provider',
	DEEPSEEK: 'deepseek-model-provider',
	GEMINI_VERTEX: 'gemini-vertex-model-provider',
	GENERIC_OPENAI: 'generic-openai-model-provider',
	AZURE: 'azure-model-provider',
	AZURE_ENTRA: 'azure-entra-model-provider'
};

export const RecommendedModelProviders = [
	CommonModelProviderIds.OPENAI,
	CommonModelProviderIds.ANTHROPIC,
	CommonModelProviderIds.AMAZON_BEDROCK,
	CommonModelProviderIds.AMAZON_BEDROCK_API_KEY,
	CommonModelProviderIds.AZURE,
	CommonModelProviderIds.AZURE_ENTRA
];

export const PROJECT_MCP_SERVER_NAME = 'MCP Servers';
export const DEFAULT_MCP_CATALOG_ID = 'default';
export const DEFAULT_SYSTEM_MCP_CATALOG_ID = 'default';

export const CommonAuthProviderIds = {
	GOOGLE: 'google-auth-provider',
	GITHUB: 'github-auth-provider',
	OKTA: 'okta-auth-provider',
	ENTRA: 'entra-auth-provider',
	AUTH0: 'auth0-auth-provider'
} as const;

export const BOOTSTRAP_USER_ID = 'bootstrap';

export const ADMIN_SESSION_STORAGE = {
	ACCESS_CONTROL_RULE_CREATION: 'access-control-rule-creation',
	LAST_VISITED_MCP_SERVER: 'last-visited-mcp-server'
} as const;

export const ADMIN_ALL_OPTION = {
	label: 'Everything in Global Registry',
	description: 'Include all MCP servers in the global registry'
};

export const MCP_PUBLISHER_ALL_OPTION = {
	label: 'Everything in My Registry',
	description: 'Include all MCP servers I have created in my registry'
};

export const ADMIN_AGENT_DISABLED_MESSAGE =
	'Set up a model provider w/ default Language Model & Language Model (Fast) models to access this page.';

export const USER_AGENT_DISABLED_MESSAGE =
	'Agent is currently disabled. Contact your administrator to enable it.';

/** Filter Constants  */
export const PII_REDACT_TYPES = 'PII_REDACT_TYPES';
export const PII_BLOCK_TYPES = 'PII_BLOCK_TYPES';

export const PII_FILTER_DEFAULT_OPTIONS = [
	{
		id: 'EMAIL_ADDRESS',
		label: 'Email Address'
	},
	{
		id: 'PHONE_NUMBER',
		label: 'Phone Number'
	},
	{
		id: 'CREDIT_CARD',
		label: 'Credit Card'
	},
	{
		id: 'CRYPTO',
		label: 'Crypto'
	},
	{
		id: 'IBAN_CODE',
		label: 'IBAN Code'
	},
	{
		id: 'IP_ADDRESS',
		label: 'IP Address'
	},
	{
		id: 'US_SSN',
		label: 'US SSN'
	},
	{
		id: 'US_BANK_NUMBER',
		label: 'US Bank Number'
	},
	{
		id: 'US_PASSPORT',
		label: 'US Passport'
	},
	{
		id: 'MEDICAL_LICENSE',
		label: 'Medical License'
	},
	{
		id: 'US_DRIVER_LICENSE',
		label: 'US Driver License'
	}
];
export const PII_FILTER_OPTIONAL_OPTIONS = [
	{
		id: 'AU_ABN',
		label: 'AU ABN'
	},
	{
		id: 'AU_ACN',
		label: 'AU ACN'
	},
	{
		id: 'AU_MEDICARE',
		label: 'AU Medicare'
	},
	{
		id: 'AU_TFN',
		label: 'AU TFN'
	},
	{
		id: 'DATE_TIME',
		label: 'Date Time'
	},
	{
		id: 'ES_NIE',
		label: 'ES NIE'
	},
	{
		id: 'ES_NIF',
		label: 'ES NIF'
	},
	{
		id: 'FI_PERSONAL_IDENTITY_CODE',
		label: 'FI Personal Identity Code'
	},
	{
		id: 'IN_AADHAAR',
		label: 'IN Aadhaar'
	},
	{
		id: 'IN_GSTIN',
		label: 'IN GSTIN'
	},
	{
		id: 'IN_PAN',
		label: 'IN PAN'
	},
	{
		id: 'IN_PASSPORT',
		label: 'IN Passport'
	},
	{
		id: 'IN_VEHICLE_REGISTRATION',
		label: 'IN Vehicle Registration'
	},
	{
		id: 'IN_VOTER',
		label: 'IN Voter'
	},
	{
		id: 'IT_DRIVER_LICENSE',
		label: 'IT Driver License'
	},
	{
		id: 'IT_FISCAL_CODE',
		label: 'IT Fiscal Code'
	},
	{
		id: 'IT_IDENTITY_CARD',
		label: 'IT Identity Card'
	},
	{
		id: 'IT_PASSPORT',
		label: 'IT Passport'
	},
	{
		id: 'IT_VAT_CODE',
		label: 'IT VAT Code'
	},
	{
		id: 'KR_BRN',
		label: 'KR BRN'
	},
	{
		id: 'KR_DRIVER_LICENSE',
		label: 'KR Driver License'
	},
	{
		id: 'KR_FRN',
		label: 'KR FRN'
	},
	{
		id: 'KR_PASSPORT',
		label: 'KR Passport'
	},
	{
		id: 'KR_RRN',
		label: 'KR RRN'
	},
	{
		id: 'LOCATION',
		label: 'Location'
	},
	{
		id: 'MAC_ADDRESS',
		label: 'MAC Address'
	},
	{
		id: 'MEDICAL_BIOLOGICAL_ATTRIBUTE',
		label: 'Medical Biological Attribute'
	},
	{
		id: 'MEDICAL_BIOLOGICAL_STRUCTURE',
		label: 'Medical Biological Structure'
	},
	{
		id: 'MEDICAL_CLINICAL_EVENT',
		label: 'Medical Clinical Event'
	},
	{
		id: 'MEDICAL_DISEASE_DISORDER',
		label: 'Medical Disease Disorder'
	},
	{
		id: 'MEDICAL_FAMILY_HISTORY',
		label: 'Medical Family History'
	},
	{
		id: 'MEDICAL_HISTORY',
		label: 'Medical History'
	},
	{
		id: 'MEDICAL_MEDICATION',
		label: 'Medical Medication'
	},
	{
		id: 'MEDICAL_THERAPEUTIC_PROCEDURE',
		label: 'Medical Therapeutic Procedure'
	},
	{
		id: 'NG_NIN',
		label: 'NG NIN'
	},
	{
		id: 'NG_VEHICLE_REGISTRATION',
		label: 'NG Vehicle Registration'
	},
	{
		id: 'NRP',
		label: 'NRP'
	},
	{
		id: 'PERSON',
		label: 'Person'
	},
	{
		id: 'PL_PESEL',
		label: 'PL PESEL'
	},
	{
		id: 'SG_NRIC_FIN',
		label: 'SG NRIC FIN'
	},
	{
		id: 'SG_UEN',
		label: 'SG UEN'
	},
	{
		id: 'TH_TNIN',
		label: 'TH TNIN'
	},
	{
		id: 'UK_NHS',
		label: 'UK NHS'
	},
	{
		id: 'UK_NINO',
		label: 'UK NINO'
	},
	{
		id: 'UK_PASSPORT',
		label: 'UK Passport'
	},
	{
		id: 'UK_POSTCODE',
		label: 'UK Postcode'
	},
	{
		id: 'UK_VEHICLE_REGISTRATION',
		label: 'UK Vehicle Registration'
	},
	{
		id: 'URL',
		label: 'URL'
	},
	{
		id: 'US_ITIN',
		label: 'US ITIN'
	},
	{
		id: 'US_MBI',
		label: 'US MBI'
	},
	{
		id: 'US_NPI',
		label: 'US NPI'
	}
];
export const PII_FILTER_OPTION_VALUES = [
	{ id: 'none', label: 'None' },
	{ id: 'block', label: 'Block' },
	{ id: 'redact', label: 'Redact' }
];
