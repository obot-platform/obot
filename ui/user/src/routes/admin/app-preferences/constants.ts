import type { AppPreferences } from '$lib/services';

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
		label: 'Full Logo'
	},
	{
		id: 'logoEnterprise',
		label: 'Full Enterprise Logo'
	},
	{
		id: 'logoChat',
		label: 'Full Chat Logo'
	}
];

export const themeDarkLogoFields: { id: keyof AppPreferences['logos']; label: string }[] = [
	{
		id: 'darkLogoDefault',
		label: 'Full Logo'
	},
	{
		id: 'darkLogoEnterprise',
		label: 'Full Enterprise Logo'
	},
	{
		id: 'darkLogoChat',
		label: 'Full Chat Logo'
	}
];

export const themeLightSurfaceFields: { id: keyof AppPreferences['theme']; label: string }[] = [
	{
		id: 'backgroundColor',
		label: 'Background'
	},
	{
		id: 'surface1Color',
		label: 'Surface 1'
	},
	{
		id: 'surface2Color',
		label: 'Surface 2'
	},
	{
		id: 'surface3Color',
		label: 'Surface 3'
	}
];

export const themeDarkSurfaceFields: { id: keyof AppPreferences['theme']; label: string }[] = [
	{
		id: 'darkBackgroundColor',
		label: 'Background'
	},
	{
		id: 'darkSurface1Color',
		label: 'Surface 1'
	},
	{
		id: 'darkSurface2Color',
		label: 'Surface 2'
	},
	{
		id: 'darkSurface3Color',
		label: 'Surface 3'
	}
];

export const themeLightIndicatorFields: { id: keyof AppPreferences['theme']; label: string }[] = [
	{
		id: 'secondaryColor',
		label: 'Secondary'
	},
	{
		id: 'successColor',
		label: 'Success'
	},
	{
		id: 'warningColor',
		label: 'Warning'
	},
	{
		id: 'errorColor',
		label: 'Error'
	}
];

export const themeDarkIndicatorFields: { id: keyof AppPreferences['theme']; label: string }[] = [
	{
		id: 'darkSecondaryColor',
		label: 'Secondary'
	},
	{
		id: 'darkSuccessColor',
		label: 'Success'
	},
	{
		id: 'darkWarningColor',
		label: 'Warning'
	},
	{
		id: 'darkErrorColor',
		label: 'Error'
	}
];

export const textLightFields: { id: keyof AppPreferences['theme']; label: string }[] = [
	{
		id: 'onBackgroundColor',
		label: 'Base Font Color'
	},
	{
		id: 'onPrimaryColor',
		label: 'Primary Button Text'
	},
	{
		id: 'onSuccessColor',
		label: 'Success Button Text'
	},
	{
		id: 'onWarningColor',
		label: 'Warning Button Text'
	},
	{
		id: 'onErrorColor',
		label: 'Error Button Text'
	}
];

export const textDarkFields: { id: keyof AppPreferences['theme']; label: string }[] = [
	{
		id: 'darkOnBackgroundColor',
		label: 'Primary Text'
	},
	{
		id: 'darkOnPrimaryColor',
		label: 'Primary Text'
	},
	{
		id: 'darkOnSuccessColor',
		label: 'Success Text'
	},
	{
		id: 'darkOnWarningColor',
		label: 'Warning Text'
	},
	{
		id: 'darkOnErrorColor',
		label: 'Error Text'
	}
];
