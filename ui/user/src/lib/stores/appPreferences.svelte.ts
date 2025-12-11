import { browser } from '$app/environment';
import type { AppPreferences } from '$lib/services';

export const DEFAULT_LOGOS = {
	// Logo.svelte variants
	icon: {
		default: '/user/images/obot-icon-blue.svg',
		error: '/user/images/obot-icon-grumpy-blue.svg',
		warning: '/user/images/obot-icon-surprised-yellow.svg'
	},
	// BetaLogo.svelte variants
	beta: {
		dark: {
			chat: '/user/images/obot-chat-logo-blue-white-text.svg',
			enterprise: '/user/images/obot-enterprise-logo-blue-white-text.svg',
			default: '/user/images/obot-logo-blue-white-text.svg'
		},
		light: {
			chat: '/user/images/obot-chat-logo-blue-black-text.svg',
			enterprise: '/user/images/obot-enterprise-logo-blue-black-text.svg',
			default: '/user/images/obot-logo-blue-black-text.svg'
		}
	}
} as const;

export function compileAppPreferences(preferences?: AppPreferences): AppPreferences {
	return {
		logos: {
			logoIcon: preferences?.logos?.logoIcon ?? DEFAULT_LOGOS.icon.default,
			logoIconError: preferences?.logos?.logoIconError ?? DEFAULT_LOGOS.icon.error,
			logoIconWarning: preferences?.logos?.logoIconWarning ?? DEFAULT_LOGOS.icon.warning,
			logoDefault: preferences?.logos?.logoDefault ?? DEFAULT_LOGOS.beta.light.default,
			logoEnterprise: preferences?.logos?.logoEnterprise ?? DEFAULT_LOGOS.beta.light.enterprise,
			logoChat: preferences?.logos?.logoChat ?? DEFAULT_LOGOS.beta.light.chat,
			darkLogoDefault: preferences?.logos?.darkLogoDefault ?? DEFAULT_LOGOS.beta.dark.default,
			darkLogoChat: preferences?.logos?.darkLogoChat ?? DEFAULT_LOGOS.beta.dark.chat,
			darkLogoEnterprise:
				preferences?.logos?.darkLogoEnterprise ?? DEFAULT_LOGOS.beta.dark.enterprise
		},
		theme: {
			backgroundColor: preferences?.theme?.backgroundColor ?? 'hsl(0 0 100)',
			onBackgroundColor: preferences?.theme?.onBackgroundColor ?? 'hsl(0 0 0)',
			onSurfaceColor: preferences?.theme?.onSurfaceColor ?? 'hsl(0 0 calc(2.5 + 60))',
			surface1Color: preferences?.theme?.surface1Color ?? 'hsl(0 0 calc(2.5 + 93))',
			surface2Color: preferences?.theme?.surface2Color ?? 'hsl(0 0 calc(2.5 + 90))',
			surface3Color: preferences?.theme?.surface3Color ?? 'hsl(0 0 calc(2.5 + 80))',
			primaryColor: preferences?.theme?.primaryColor ?? '#4f7ef3',
			darkBackgroundColor: preferences?.theme?.darkBackgroundColor ?? 'hsl(0 0 0)',
			darkOnBackgroundColor: preferences?.theme?.darkOnBackgroundColor ?? 'hsl(0 0 calc(2.5 + 95))',
			darkOnSurfaceColor: preferences?.theme?.darkOnSurfaceColor ?? 'hsl(0 0 calc(2.5 + 50))',
			darkSurface1Color: preferences?.theme?.darkSurface1Color ?? 'hsl(0 0 calc(2.5 + 5))',
			darkSurface2Color: preferences?.theme?.darkSurface2Color ?? 'hsl(0 0 calc(2.5 + 10))',
			darkSurface3Color: preferences?.theme?.darkSurface3Color ?? 'hsl(0 0 calc(2.5 + 20))',
			darkPrimaryColor: preferences?.theme?.darkPrimaryColor ?? '#4f7ef3'
		}
	};
}

const store = $state<{
	current: AppPreferences;
	loaded: boolean;
	setThemeColors: (colors: AppPreferences['theme']) => void;
	initialize: (preferences: AppPreferences) => void;
}>({
	current: compileAppPreferences(),
	loaded: false,
	setThemeColors,
	initialize
});

function setThemeColors(colors: AppPreferences['theme']) {
	// Set light theme colors
	document.documentElement.style.setProperty('--theme-background-light', colors.backgroundColor);
	document.documentElement.style.setProperty(
		'--theme-on-background-light',
		colors.onBackgroundColor
	);
	document.documentElement.style.setProperty('--theme-on-surface-light', colors.onSurfaceColor);
	document.documentElement.style.setProperty('--theme-surface1-light', colors.surface1Color);
	document.documentElement.style.setProperty('--theme-surface2-light', colors.surface2Color);
	document.documentElement.style.setProperty('--theme-surface3-light', colors.surface3Color);
	document.documentElement.style.setProperty('--theme-primary-light', colors.primaryColor);

	// Set dark theme colors
	document.documentElement.style.setProperty('--theme-background-dark', colors.darkBackgroundColor);
	document.documentElement.style.setProperty(
		'--theme-on-background-dark',
		colors.darkOnBackgroundColor
	);
	document.documentElement.style.setProperty('--theme-on-surface-dark', colors.darkOnSurfaceColor);
	document.documentElement.style.setProperty('--theme-surface1-dark', colors.darkSurface1Color);
	document.documentElement.style.setProperty('--theme-surface2-dark', colors.darkSurface2Color);
	document.documentElement.style.setProperty('--theme-surface3-dark', colors.darkSurface3Color);
	document.documentElement.style.setProperty('--theme-primary-dark', colors.darkPrimaryColor);
}

function initialize(preferences: AppPreferences) {
	store.current = preferences;
	store.loaded = true;
	if (browser) {
		store.setThemeColors(store.current.theme);
	}
}

export default store;
