import type { AppPreferences } from '$lib/services';

const LOGO_LABELS = {
	default: 'Full Logo',
	enterprise: 'Full Enterprise Logo',
	chat: 'Full Chat Logo'
};

const INDICATOR_LABELS = {
	secondary: 'Secondary',
	success: 'Success',
	warning: 'Warning',
	error: 'Error'
};

const TEXT_LABELS = {
	base: 'Base Font Color',
	onAccent: 'On-Accent Button Text',
	success: 'Success Button Text',
	warning: 'Warning Button Text',
	error: 'Error Button Text'
};

const SURFACE_LABELS = {
	background: 'Background',
	surface1: 'Surface 1',
	surface2: 'Surface 2',
	surface3: 'Surface 3'
};

export type BrandingMockConnectorRow = {
	id: string;
	name: string;
	devicon: string;
	type: string;
	status: string;
	created: string;
	registry: string;
	users: number;
};

export const MOCK_CONNECTOR_TABLE_DATA: BrandingMockConnectorRow[] = [
	{
		id: 'mock-braintree',
		name: 'Braintree MCP',
		devicon: 'devicon-python-plain',
		type: 'single',
		status: 'Connected',
		created: new Date(Date.now() - 1000 * 60 * 45).toISOString(),
		registry: 'Global Registry',
		users: 1
	},
	{
		id: 'mock-acme-api',
		name: 'Acme Remote API',
		devicon: 'devicon-typescript-plain',
		type: 'remote',
		status: 'Requires OAuth Config',
		created: new Date(Date.now() - 1000 * 60 * 60 * 20).toISOString(),
		registry: 'Global Registry',
		users: 0
	},
	{
		id: 'mock-analytics',
		name: 'Analytics Warehouse',
		devicon: 'devicon-postgresql-plain',
		type: 'multi',
		status: 'Connected',
		created: new Date(Date.now() - 1000 * 60 * 60 * 24 * 3).toISOString(),
		registry: 'My Registry',
		users: 12
	},
	{
		id: 'mock-compose',
		name: 'Composite Toolkit',
		devicon: 'devicon-docker-plain',
		type: 'composite',
		status: '',
		created: new Date(Date.now() - 1000 * 60 * 60 * 24 * 14).toISOString(),
		registry: 'Global Registry',
		users: 4
	},
	{
		id: 'mock-slack',
		name: 'Slack Connector',
		devicon: 'devicon-slack-plain',
		type: 'remote',
		status: '',
		created: new Date(Date.now() - 1000 * 60 * 60 * 24 * 30).toISOString(),
		registry: "Partner's Registry",
		users: 0
	},
	{
		id: 'mock-react',
		name: 'UI Automation Server',
		devicon: 'devicon-react-original',
		type: 'single',
		status: 'Connected',
		created: new Date(Date.now() - 1000 * 60 * 8).toISOString(),
		registry: 'Global Registry',
		users: 2
	}
];

export const standardIconFields: { id: keyof AppPreferences['logos']; label: string }[] = [
	{
		id: 'logoIcon',
		label: 'Default Icon'
	},
	{
		id: 'logoIconError',
		label: 'Error Icon'
	},
	{
		id: 'logoIconWarning',
		label: 'Warning Icon'
	}
];

export const themeLightLogoFields: { id: keyof AppPreferences['logos']; label: string }[] = [
	{
		id: 'logoDefault',
		label: LOGO_LABELS.default
	},
	{
		id: 'logoEnterprise',
		label: LOGO_LABELS.enterprise
	},
	{
		id: 'logoChat',
		label: LOGO_LABELS.chat
	}
];

export const themeDarkLogoFields: { id: keyof AppPreferences['logos']; label: string }[] = [
	{
		id: 'darkLogoDefault',
		label: LOGO_LABELS.default
	},
	{
		id: 'darkLogoEnterprise',
		label: LOGO_LABELS.enterprise
	},
	{
		id: 'darkLogoChat',
		label: LOGO_LABELS.chat
	}
];

export const themeLightSurfaceFields: { id: keyof AppPreferences['theme']; label: string }[] = [
	{
		id: 'backgroundColor',
		label: SURFACE_LABELS.background
	},
	{
		id: 'surface1Color',
		label: SURFACE_LABELS.surface1
	},
	{
		id: 'surface2Color',
		label: SURFACE_LABELS.surface2
	},
	{
		id: 'surface3Color',
		label: SURFACE_LABELS.surface3
	}
];

export const themeDarkSurfaceFields: { id: keyof AppPreferences['theme']; label: string }[] = [
	{
		id: 'darkBackgroundColor',
		label: SURFACE_LABELS.background
	},
	{
		id: 'darkSurface1Color',
		label: SURFACE_LABELS.surface1
	},
	{
		id: 'darkSurface2Color',
		label: SURFACE_LABELS.surface2
	},
	{
		id: 'darkSurface3Color',
		label: SURFACE_LABELS.surface3
	}
];

export const themeLightIndicatorFields: { id: keyof AppPreferences['theme']; label: string }[] = [
	{
		id: 'secondaryColor',
		label: INDICATOR_LABELS.secondary
	},
	{
		id: 'successColor',
		label: INDICATOR_LABELS.success
	},
	{
		id: 'warningColor',
		label: INDICATOR_LABELS.warning
	},
	{
		id: 'errorColor',
		label: INDICATOR_LABELS.error
	}
];

export const themeDarkIndicatorFields: { id: keyof AppPreferences['theme']; label: string }[] = [
	{
		id: 'darkSecondaryColor',
		label: INDICATOR_LABELS.secondary
	},
	{
		id: 'darkSuccessColor',
		label: INDICATOR_LABELS.success
	},
	{
		id: 'darkWarningColor',
		label: INDICATOR_LABELS.warning
	},
	{
		id: 'darkErrorColor',
		label: INDICATOR_LABELS.error
	}
];

export const textLightFields: { id: keyof AppPreferences['theme']; label: string }[] = [
	{
		id: 'onBackgroundColor',
		label: TEXT_LABELS.base
	},
	{
		id: 'onPrimaryColor',
		label: TEXT_LABELS.onAccent
	},
	{
		id: 'onSuccessColor',
		label: TEXT_LABELS.success
	},
	{
		id: 'onWarningColor',
		label: TEXT_LABELS.warning
	},
	{
		id: 'onErrorColor',
		label: TEXT_LABELS.error
	}
];

export const textDarkFields: { id: keyof AppPreferences['theme']; label: string }[] = [
	{
		id: 'darkOnBackgroundColor',
		label: TEXT_LABELS.base
	},
	{
		id: 'darkOnPrimaryColor',
		label: TEXT_LABELS.onAccent
	},
	{
		id: 'darkOnSuccessColor',
		label: TEXT_LABELS.success
	},
	{
		id: 'darkOnWarningColor',
		label: TEXT_LABELS.warning
	},
	{
		id: 'darkOnErrorColor',
		label: TEXT_LABELS.error
	}
];
