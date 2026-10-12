import { m } from '$lib/i18n';

export const ABORTED_THREAD_MESSAGE = 'thread was aborted, cancelling run';
export const ABORTED_BY_USER_MESSAGE = 'aborted by user';

export const UNAUTHORIZED_PATHS = new Set([
	'/',
	'/privacy-policy',
	'/terms-of-service',
	'/admin',
	// The local auth provider's login form: anonymous by definition, so a 401 from the layout's
	// profile fetch must not bounce the user back to the provider list.
	'/login/local',
	// Activation carries its setup token in the URL fragment, so redirecting an anonymous visitor
	// away would discard the only browser-side copy of it.
	'/activate',
	// The login page of the Okta SCIM provisioning app, which explains to anyone who opens the app
	// that it does not sign them in. Okta requires a separate app just to act as a SCIM provisioning
	// container, and it needs a login URL, even though it will never be used for login. So we point
	// it to this page.
	'/okta-scim'
]);

export const PAGE_TRANSITION_DURATION = 200;
export const PAGE_SIZE = 50;

// The sub-tabs of the Auth Providers tab of Identity & Access: the providers, and SCIM.
export const AUTH_PROVIDERS_VIEW_PATH = '/identity-access?view=auth-providers';
export const SCIM_VIEW_PATH = '/identity-access?view=auth-providers&subview=scim';
const scimViewParams = Array.from(new URL(SCIM_VIEW_PATH, 'http://localhost').searchParams);

// Whether the query of a URL of Identity & Access selects the SCIM sub-tab, as SCIM_VIEW_PATH's does.
export function isSCIMView(searchParams: URLSearchParams): boolean {
	return scimViewParams.every(([key, value]) => searchParams.get(key) === value);
}

export const SEEN_SPLASH_DIALOG_KEY = 'seenSplashDialog';

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
export const MCP_NAV_SOURCE_PARAM = 'from';
export const MCP_CONNECTORS_NAV_SOURCE = 'connectors';
export const DEFAULT_SYSTEM_MCP_CATALOG_ID = 'default';

export const CommonAuthProviderIds = {
	GOOGLE: 'google-auth-provider',
	GITHUB: 'github-auth-provider',
	OKTA: 'okta-auth-provider',
	ENTRA: 'entra-auth-provider',
	AUTH0: 'auth0-auth-provider',
	JUMPCLOUD: 'jumpcloud-auth-provider',
	LOCAL: 'local-auth-provider'
} as const;

/** Matches localauth.MinPasswordLength on the server. */
export const LOCAL_AUTH_MIN_PASSWORD_LENGTH = 12;

export const BOOTSTRAP_USER_ID = 'bootstrap';

export const ADMIN_SESSION_STORAGE = {
	LAST_VISITED_MCP_SERVER: 'last-visited-mcp-server'
} as const;

export const ADMIN_ALL_OPTION = {
	label: m.core_everything_in_global_registry(),
	description: m.core_everything_in_global_registry_description()
};

export const MCP_PUBLISHER_ALL_OPTION = {
	label: m.core_everything_in_my_registry(),
	description: m.core_everything_in_my_registry_description()
};

/** Filter Constants  */
export const PII_REDACT_TYPES = 'PII_REDACT_TYPES';
export const PII_BLOCK_TYPES = 'PII_BLOCK_TYPES';

export const PII_FILTER_DEFAULT_OPTIONS = [
	{
		id: 'EMAIL_ADDRESS',
		label: m.mcps_filters_personal_data_email_address()
	},
	{
		id: 'PHONE_NUMBER',
		label: m.mcps_filters_personal_data_phone_number()
	},
	{
		id: 'CREDIT_CARD',
		label: m.mcps_filters_personal_data_credit_card()
	},
	{
		id: 'CRYPTO',
		label: m.mcps_filters_personal_data_crypto()
	},
	{
		id: 'IBAN_CODE',
		label: m.mcps_filters_personal_data_iban_code()
	},
	{
		id: 'IP_ADDRESS',
		label: m.mcps_filters_personal_data_ip_address()
	},
	{
		id: 'US_SSN',
		label: 'US SSN'
	},
	{
		id: 'US_BANK_NUMBER',
		label: m.mcps_filters_personal_data_us_bank_number()
	},
	{
		id: 'US_PASSPORT',
		label: m.mcps_filters_personal_data_us_passport()
	},
	{
		id: 'MEDICAL_LICENSE',
		label: m.mcps_filters_personal_data_medical_license()
	},
	{
		id: 'US_DRIVER_LICENSE',
		label: m.mcps_filters_personal_data_us_driver_license()
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
		label: m.mcps_filters_personal_data_au_medicare()
	},
	{
		id: 'AU_TFN',
		label: 'AU TFN'
	},
	{
		id: 'DATE_TIME',
		label: m.mcps_filters_personal_data_date_time()
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
		label: m.mcps_filters_personal_data_fi_personal_identity_code()
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
		label: m.mcps_filters_personal_data_in_passport()
	},
	{
		id: 'IN_VEHICLE_REGISTRATION',
		label: m.mcps_filters_personal_data_in_vehicle_registration()
	},
	{
		id: 'IN_VOTER',
		label: m.mcps_filters_personal_data_in_voter()
	},
	{
		id: 'IT_DRIVER_LICENSE',
		label: m.mcps_filters_personal_data_it_driver_license()
	},
	{
		id: 'IT_FISCAL_CODE',
		label: m.mcps_filters_personal_data_it_fiscal_code()
	},
	{
		id: 'IT_IDENTITY_CARD',
		label: m.mcps_filters_personal_data_it_identity_card()
	},
	{
		id: 'IT_PASSPORT',
		label: m.mcps_filters_personal_data_it_passport()
	},
	{
		id: 'IT_VAT_CODE',
		label: m.mcps_filters_personal_data_it_vat_code()
	},
	{
		id: 'KR_BRN',
		label: 'KR BRN'
	},
	{
		id: 'KR_DRIVER_LICENSE',
		label: m.mcps_filters_personal_data_kr_driver_license()
	},
	{
		id: 'KR_FRN',
		label: 'KR FRN'
	},
	{
		id: 'KR_PASSPORT',
		label: m.mcps_filters_personal_data_kr_passport()
	},
	{
		id: 'KR_RRN',
		label: 'KR RRN'
	},
	{
		id: 'LOCATION',
		label: m.mcps_filters_personal_data_location()
	},
	{
		id: 'MAC_ADDRESS',
		label: m.mcps_filters_personal_data_mac_address()
	},
	{
		id: 'MEDICAL_BIOLOGICAL_ATTRIBUTE',
		label: m.mcps_filters_personal_data_medical_biological_attribute()
	},
	{
		id: 'MEDICAL_BIOLOGICAL_STRUCTURE',
		label: m.mcps_filters_personal_data_medical_biological_structure()
	},
	{
		id: 'MEDICAL_CLINICAL_EVENT',
		label: m.mcps_filters_personal_data_medical_clinical_event()
	},
	{
		id: 'MEDICAL_DISEASE_DISORDER',
		label: m.mcps_filters_personal_data_medical_disease_disorder()
	},
	{
		id: 'MEDICAL_FAMILY_HISTORY',
		label: m.mcps_filters_personal_data_medical_family_history()
	},
	{
		id: 'MEDICAL_HISTORY',
		label: m.mcps_filters_personal_data_medical_history()
	},
	{
		id: 'MEDICAL_MEDICATION',
		label: m.mcps_filters_personal_data_medical_medication()
	},
	{
		id: 'MEDICAL_THERAPEUTIC_PROCEDURE',
		label: m.mcps_filters_personal_data_medical_therapeutic_procedure()
	},
	{
		id: 'NG_NIN',
		label: 'NG NIN'
	},
	{
		id: 'NG_VEHICLE_REGISTRATION',
		label: m.mcps_filters_personal_data_ng_vehicle_registration()
	},
	{
		id: 'NRP',
		label: 'NRP'
	},
	{
		id: 'PERSON',
		label: m.mcps_filters_personal_data_person()
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
		label: m.mcps_filters_personal_data_uk_passport()
	},
	{
		id: 'UK_POSTCODE',
		label: m.mcps_filters_personal_data_uk_postcode()
	},
	{
		id: 'UK_VEHICLE_REGISTRATION',
		label: m.mcps_filters_personal_data_uk_vehicle_registration()
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
	{ id: 'none', label: m.core_col_none() },
	{ id: 'block', label: m.mcps_filters_personal_data_option_block() },
	{ id: 'redact', label: m.mcps_filters_personal_data_option_redact() }
];

export const OBOT_GUIDE_KEYS = {
	SHOW_ALL_GUIDES: '@obot/show-all-guides'
} as const;

export const AI_CLIENT_PREFERENCE_KEY = 'aiClientPreference';

// IDs
export const CATALOG_SERVER_FIELD_IDS = {
	serverFormDetails: 'catalog-server-form-details',
	name: 'catalog-server-name',
	description: 'catalog-server-description-label',
	descriptionHint: 'catalog-server-description-hint',
	shortDescription: 'catalog-server-short-description',
	shortDescriptionHint: 'catalog-server-short-description-hint',
	shortDescriptionCount: 'catalog-server-short-description-count',
	shortDescriptionError: 'catalog-server-short-description-error',
	icon: 'catalog-server-icon',
	serverType: 'catalog-server-tenancy-type-label',
	serverTypeHint: 'catalog-server-tenancy-hint',
	nameError: 'catalog-server-name-error',
	formError: 'catalog-server-form-error',
	tenancy: 'catalog-server-tenancy',
	runtime: 'catalog-server-runtime',
	runtimeConfiguration: 'catalog-server-runtime-configuration',
	configuration: 'catalog-server-configuration',
	addConfigurationBtn: 'catalog-server-add-configuration-btn',
	env: 'catalog-server-env',
	header: 'catalog-server-header',
	headers: 'catalog-server-user-headers',
	remoteURL: 'catalog-server-remote-url',
	remoteAdvancedBtn: 'catalog-server-remote-advanced-btn',
	remoteConnection: 'catalog-server-remote-connection',
	remoteHeaders: 'catalog-server-remote-headers',
	remoteStaticOAuth: 'catalog-server-remote-static-oauth',
	compositeEntries: 'catalog-server-composite-entries',
	addCompositeEntryBtn: 'catalog-server-add-composite-entry-btn',
	submitBtn: 'catalog-server-form-submit',
	cancelBtn: 'catalog-server-form-cancel',
	removeConfigurationBtn: 'catalog-server-remove-configuration-btn',
	compositeEntryChoice: 'catalog-server-composite-entry-choice',
	compositeEntrySearchMcpServersDialog: 'search-mcp-servers-dialog',
	compositeEntrySearchMcpServersConfirmBtn: 'search-mcp-servers-confirm-btn',
	compositeEntrySearchMcpServersCancelBtn: 'search-mcp-servers-cancel-btn',
	compositeEntrySkipBtn: 'composite-entry-choice-skip-btn',
	compositeEntryConfigureToolsBtn: 'composite-entry-choice-configure-tools-btn',
	compositeEntryConfigureToolsGetStartedBtn:
		'composite-entry-choice-configure-tools-get-started-btn',
	compositeEntryConfigureToolsToggleAll: 'composite-entry-choice-configure-tools-toggle-all',
	compositeEntryConfigureToolsConfirmBtn: 'composite-entry-choice-configure-tools-confirm-btn',
	compositeEntryToolCollapseBtn: 'composite-entry-tool-collapse-btn',
	compositeEntryEditToolsDialog: 'composite-entry-edit-tools-dialog'
};

export const MCP_ACCESS_POLICY_FIELD_IDS = {
	addPolicyBtn: 'mcp-access-policy-add-btn',
	addPolicyEmptyBtn: 'mcp-access-policy-empty-add-btn',
	name: 'mcp-access-policy-name',
	usersGroupsSection: 'mcp-access-policy-users-groups-section',
	addUserGroupBtn: 'mcp-access-policy-add-user-group-btn',
	allUsersOption: 'mcp-access-policy-all-users-option',
	userGroupConfirmBtn: 'mcp-access-policy-user-group-confirm-btn',
	serversSection: 'mcp-access-policy-servers-section',
	addServerBtn: 'mcp-access-policy-add-server-btn',
	everythingOption: 'mcp-access-policy-everything-option',
	serverConfirmBtn: 'mcp-access-policy-server-confirm-btn',
	saveBtn: 'mcp-access-policy-save-btn'
} as const;

export const MDM_DEVICES_CONFIGURATION_FIELD_IDS = {
	devicesLink: 'sidebar-link-inventory',
	enforcementEventsLink: 'sidebar-link-enforcement-events',
	configurationTab: 'tab-configuration',
	inventoryTabDeviceMcpServers: 'tab-device-mcp-servers',
	configurationDetails: 'devices-configuration-details',
	getStartedButton: 'devices-configuration-get-started',
	newEnrollmentKeyButton: 'enrollment-new-key-btn',
	newEnrollmentKeyDialog: 'new-enrollment-key-dialog',
	enrollmentKeyButton: 'enrollment-key-btn',
	enrollmentConfigSetup: 'enrollment-config-setup',
	enrollmentConfigSetupStep: 'enrollment-config-setup-step',
	enrollmentKeysSection: 'enrollment-keys-section',
	operatingSystemStep: 'devices-install-operating-system',
	downloadStep: 'devices-install-download',
	installInstructionsStep: 'devices-install-instructions',
	agentSettingsButton: 'devices-agent-settings',
	checkForUpdatesButton: 'devices-check-for-updates-button',
	devicesTabOverview: 'tab-overview',
	devicesTabDevices: 'tab-devices',
	toolCallEnforcementSection: 'tool-call-enforcement-section'
};

export const MCP_FILTERS_FIELD_IDS = {
	addFilterBtn: 'filter-add-button',
	createCustomBtn: 'filter-create-custom-button',
	createBuiltInBtn: 'filter-create-built-in-button',
	basicDetails: 'filter-basic-details',
	runtimeFormDetails: 'filter-runtime-form-details',
	filterSelectors: 'filter-selectors',
	filterMcpServers: 'filter-mcp-servers',
	runtimeSelector: 'filter-runtime-selector',
	saveBtn: 'filter-save-btn'
};

export const CLOUD_ENTITLEMENT = 'OBOT_CLOUD';
export const COMMUNITY_ENTITLEMENT = 'OBOT_COMMUNITY';
export const ENTERPRISE_ENTITLEMENT = 'OBOT_ENTERPRISE';
export const MODEL_PROVIDERS_ENTITLEMENT = 'OBOT_ENTERPRISE_MODEL_PROVIDERS';

export const COMMUNITY_SIGNUP_BANNER_COPY = m.core_community_signup_banner();
