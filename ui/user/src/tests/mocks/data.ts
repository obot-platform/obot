import type { AdminService, UserService } from '$lib/services';
import { faker } from '@faker-js/faker';

/**
 * This file contains mocked response data for API endpoints that are utilized in the operation.ts files of the "src/lib/services" directory.
 *
 * When adding a mocked response, keep the responses grouped by type and sorted alphabetically.
 * The mocked variable name should be the same as the function name in the operation.ts file.
 * When mocking the data, utilize the openapi_generated.go file to determine the expected response shape, and utilize the faker library to generate the mocked data if appropriate.
 */

const userID = faker.string.numeric();

// App notification

export const getAppNotificationResponse = {
	banner: {
		dismissible: false,
		enabled: false,
		resetDismissed: false,
		text: '',
		type: 'info'
	}
} satisfies Awaited<ReturnType<typeof UserService.getAppNotification>>;

// App preferences

export const listAppPreferencesResponse = {
	logos: {
		darkLogoChat: '/user/images/obot-chat-logo-blue-white-text.svg',
		darkLogoCommunity: '/user/images/obot-community-logo-blue-white-text.svg',
		darkLogoDefault: '/user/images/obot-logo-blue-white-text.svg',
		darkLogoEnterprise: '/user/images/obot-enterprise-logo-blue-white-text.svg',
		logoChat: '/user/images/obot-chat-logo-blue-black-text.svg',
		logoCommunity: '/user/images/obot-community-logo-blue-black-text.svg',
		logoDefault: '/user/images/obot-logo-blue-black-text.svg',
		logoEnterprise: '/user/images/obot-enterprise-logo-blue-black-text.svg',
		logoIcon: '/user/images/obot-icon-blue.svg',
		logoIconError: '/user/images/obot-icon-grumpy-blue.svg',
		logoIconWarning: '/user/images/obot-icon-surprised-yellow.svg'
	},
	theme: {
		backgroundColor: 'hsl(0 0 100)',
		darkBackgroundColor: 'hsl(0 0 0)',
		darkErrorColor: '#ef4444',
		darkOnBackgroundColor: 'hsl(0 0 97.5)',
		darkOnErrorColor: 'hsl(0 0 97.5)',
		darkOnPrimaryColor: 'hsl(0 0 97.5)',
		darkOnSuccessColor: 'hsl(0 0 97.5)',
		darkOnWarningColor: 'hsl(0 0 97.5)',
		darkPrimaryColor: '#4f7ef3',
		darkSecondaryColor: 'hsl(0 0 22.5)',
		darkSuccessColor: 'oklch(67% 0.13 149)',
		darkSurface1Color: 'hsl(0 0 7.5)',
		darkSurface2Color: 'hsl(0 0 12.5)',
		darkSurface3Color: 'hsl(0 0 22.5)',
		darkWarningColor: 'oklch(79.5% 0.184 86.047)',
		errorColor: '#ef4444',
		fontFamily: 'Poppins, ui-sans-serif, system-ui, sans-serif',
		onBackgroundColor: 'hsl(0 0 0)',
		onErrorColor: 'hsl(0 0 100)',
		onPrimaryColor: 'hsl(0 0 100)',
		onSuccessColor: 'hsl(0 0 100)',
		onWarningColor: 'hsl(0 0 100)',
		primaryColor: '#4f7ef3',
		secondaryColor: 'hsl(0 0 82.5)',
		successColor: 'oklch(67% 0.13 149)',
		surface1Color: 'hsl(0 0 95.5)',
		surface2Color: 'hsl(0 0 92.5)',
		surface3Color: 'hsl(0 0 82.5)',
		warningColor: 'oklch(79.5% 0.184 86.047)'
	}
} satisfies Awaited<ReturnType<typeof UserService.listAppPreferences>>;

// Auth providers

export const listAuthProvidersResponse = [
	{
		id: 'google-auth-provider',
		created: '2026-08-04T16:58:40-04:00',
		type: 'authprovider',
		name: 'Google',
		icon: '/admin/assets/google_icon_small.png',
		image: '',
		port: 0,
		link: 'https://google.com/',
		requiredConfigurationParameters: [
			{
				name: 'OBOT_GOOGLE_AUTH_PROVIDER_CLIENT_ID',
				friendlyName: 'Client ID',
				description:
					"Unique identifier for the application when using Google's OAuth. Can typically be found in Google Cloud Console \u003e Credentials"
			},
			{
				name: 'OBOT_GOOGLE_AUTH_PROVIDER_CLIENT_SECRET',
				friendlyName: 'Client Secret',
				description:
					"Password or key that app uses to authenticate with Google's OAuth. Can typically be found in Google Cloud Console \u003e Credentials",
				sensitive: true
			},
			{
				name: 'OBOT_AUTH_PROVIDER_COOKIE_SECRET',
				friendlyName: 'Cookie Secret',
				description:
					'Secret used to encrypt cookies. Must be a random string of length 16, 24, or 32.',
				sensitive: true,
				hidden: true
			},
			{
				name: 'OBOT_AUTH_PROVIDER_EMAIL_DOMAINS',
				friendlyName: 'Allowed E-Mail Domains',
				description:
					'A list of email domains that are allowed to authenticate with this provider. * is a special value that allows all domains.'
			}
		],
		optionalConfigurationParameters: [
			{
				name: 'OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN',
				friendlyName: 'PostgreSQL connection string (DSN)',
				description:
					'The connection string for a PostgreSQL database to use for session storage. If unset, cookies will be used for session storage instead.',
				sensitive: true,
				hidden: true
			},
			{
				name: 'OBOT_AUTH_PROVIDER_POSTGRES_MAX_CONNECTIONS',
				friendlyName: 'PostgreSQL max connections',
				description: 'The maximum number of open connections to the PostgreSQL database.',
				hidden: true
			},
			{
				name: 'OBOT_AUTH_PROVIDER_POSTGRES_MAX_IDLE_CONNECTIONS',
				friendlyName: 'PostgreSQL max idle connections',
				description: 'The maximum number of idle connections to the PostgreSQL database.',
				hidden: true
			},
			{
				name: 'OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_LIFETIME_SECONDS',
				friendlyName: 'PostgreSQL connection lifetime',
				description: 'The maximum lifetime of a connection to the PostgreSQL database.',
				hidden: true
			},
			{
				name: 'OBOT_AUTH_PROVIDER_TOKEN_REFRESH_DURATION',
				friendlyName: 'Token Refresh Duration',
				description:
					'Time to wait before attempting to refresh auth tokens. Should be in a format like 1h1m1s. Default: 1h'
			},
			{
				name: 'OBOT_AUTH_PROVIDER_ENABLE_LOGGING',
				friendlyName: 'Enable Logging',
				description:
					'Set to true to enable request, auth, and standard logging for the auth provider. Default: false'
			}
		],
		configured: false,
		missingConfigurationParameters: [
			'OBOT_GOOGLE_AUTH_PROVIDER_CLIENT_ID',
			'OBOT_GOOGLE_AUTH_PROVIDER_CLIENT_SECRET',
			'OBOT_AUTH_PROVIDER_COOKIE_SECRET',
			'OBOT_AUTH_PROVIDER_EMAIL_DOMAINS'
		],
		namespace: 'default'
	},
	{
		id: 'entra-auth-provider',
		created: '2026-08-04T16:58:40-04:00',
		type: 'authprovider',
		name: 'Microsoft Entra',
		icon: '/admin/assets/entra_icon.svg',
		image: '',
		port: 0,
		link: 'https://entra.microsoft.com/',
		missingEntitlements: ['OBOT_ENTERPRISE_AUTH_PROVIDERS'],
		requiredConfigurationParameters: [
			{
				name: 'OBOT_ENTRA_AUTH_PROVIDER_CLIENT_ID',
				friendlyName: 'Client ID',
				description:
					'Client ID for your Microsoft Entra OAuth app. Can be found in Microsoft Entra Admin Center \u003e App registrations'
			},
			{
				name: 'OBOT_ENTRA_AUTH_PROVIDER_CLIENT_SECRET',
				friendlyName: 'Client Secret',
				description:
					'Client secret for your Microsoft Entra OAuth app. Can be found in Microsoft Entra Admin Center \u003e App registrations',
				sensitive: true
			},
			{
				name: 'OBOT_ENTRA_AUTH_PROVIDER_TENANT_ID',
				friendlyName: 'Tenant ID',
				description:
					'Tenant ID for your Microsoft Entra tenant. Can be found in Microsoft Entra Admin Center \u003e Overview'
			},
			{
				name: 'OBOT_AUTH_PROVIDER_COOKIE_SECRET',
				friendlyName: 'Cookie Secret',
				description:
					'Secret used to encrypt cookies. Must be a random string of length 16, 24, or 32.',
				sensitive: true,
				hidden: true
			},
			{
				name: 'OBOT_AUTH_PROVIDER_EMAIL_DOMAINS',
				friendlyName: 'Allowed E-Mail Domains',
				description:
					'A list of email domains that are allowed to authenticate with this provider. * is a special value that allows all domains.'
			}
		],
		optionalConfigurationParameters: [
			{
				name: 'OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN',
				friendlyName: 'PostgreSQL connection string (DSN)',
				description:
					'The connection string for a PostgreSQL database to use for session storage. If unset, cookies will be used for session storage instead.',
				sensitive: true,
				hidden: true
			},
			{
				name: 'OBOT_AUTH_PROVIDER_POSTGRES_MAX_CONNECTIONS',
				friendlyName: 'PostgreSQL max open connections',
				description: 'The maximum number of open connections to the PostgreSQL database.',
				hidden: true
			},
			{
				name: 'OBOT_AUTH_PROVIDER_POSTGRES_MAX_IDLE_CONNECTIONS',
				friendlyName: 'PostgreSQL max idle connections',
				description: 'The maximum number of idle connections to the PostgreSQL database.',
				hidden: true
			},
			{
				name: 'OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_LIFETIME_SECONDS',
				friendlyName: 'PostgreSQL connection lifetime',
				description: 'The maximum lifetime of a PostgreSQL database connection, in seconds.',
				hidden: true
			},
			{
				name: 'OBOT_AUTH_PROVIDER_ENABLE_LOGGING',
				friendlyName: 'Enable Logging',
				description:
					'Set to true to enable request, auth, and standard logging for the auth provider. Default: false'
			}
		],
		configured: true,
		namespace: 'default'
	}
] satisfies Awaited<ReturnType<typeof AdminService.listAuthProviders>>;

// License

export const getLicenseResponse = {
	entitlements: null,
	enterprise: false,
	licenseKey: '',
	locked: false,
	source: ''
} satisfies Awaited<ReturnType<typeof UserService.getLicense>>;

// MCP servers

export const createMCPCatalogEntryResponse = {
	id: faker.string.uuid(),
	created: faker.date.recent().toISOString(),
	manifest: {
		name: 'Test Catalog Server',
		shortDescription: 'A catalog server used in tests',
		description: '',
		icon: '',
		runtime: 'npx',
		serverUserType: 'singleUser',
		npxConfig: {
			package: '@modelcontextprotocol/server-everything',
			args: [],
			egressDomains: []
		},
		env: [
			{
				key: 'TEST_API_KEY',
				name: 'Test API Key',
				description: '',
				required: false,
				sensitive: false,
				value: '',
				file: false
			}
		]
	},
	type: 'mcpservercatalogentry',
	isCatalogEntry: true
} satisfies Awaited<ReturnType<typeof AdminService.createMCPCatalogEntry>>;

export const listMCPCatalogServersResponse = [] satisfies Awaited<
	ReturnType<typeof UserService.listMCPCatalogServers>
>;

export const listMCPsResponse = [] satisfies Awaited<ReturnType<typeof UserService.listMCPs>>;

export const listMcpServerInstancesResponse = [] satisfies Awaited<
	ReturnType<typeof UserService.listMcpServerInstances>
>;

export const listSingleOrRemoteMcpServersResponse = [] satisfies Awaited<
	ReturnType<typeof UserService.listSingleOrRemoteMcpServers>
>;

// Models

export const listDefaultModelAliasesResponse = [] satisfies Awaited<
	ReturnType<typeof UserService.listDefaultModelAliases>
>;

export const listModelsResponse = [] satisfies Awaited<ReturnType<typeof UserService.listModels>>;

// Profile

export const getProfileResponse = {
	effectiveRole: 1,
	email: faker.internet.email(),
	groups: [],
	iconURL: faker.image.url(),
	id: userID,
	role: 1,
	username: faker.internet.username()
} satisfies Awaited<ReturnType<typeof UserService.getProfile>>;

// Users

export const listUsersResponse = [
	{
		created: faker.date.past().toISOString(),
		effectiveRole: getProfileResponse.effectiveRole,
		email: getProfileResponse.email,
		explicitRole: true,
		groups: getProfileResponse.groups,
		iconURL: getProfileResponse.iconURL,
		id: userID,
		role: getProfileResponse.role,
		username: getProfileResponse.username
	}
] satisfies Awaited<ReturnType<typeof UserService.listUsers>>;

// Version

export const getVersionResponse = {
	agentsEnabled: true,
	authEnabled: true,
	engine: 'docker',
	enterprise: false,
	hideK8sDetails: false,
	latestVersion: 'v0.0.0-dev',
	licenseEntitlementViolations: undefined,
	licenseEntitlements: undefined,
	mcpDefaultDenyAllEgress: false,
	mcpNetworkPolicyEnabled: false,
	messagePoliciesEnabled: false,
	missingLicenseEntitlements: [],
	obot: 'v0.0.0-dev',
	sessionStore: 'cookie',
	upgradeAvailable: false
} satisfies Awaited<ReturnType<typeof UserService.getVersion>>;
