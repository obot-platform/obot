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

/** Default UI font stack; kept in sync with `app.css` (`--theme-font-family` fallback). */
export const DEFAULT_FONT_FAMILY =
	'Poppins, ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, Helvetica Neue, Arial, Noto Sans, sans-serif, Apple Color Emoji, Segoe UI Emoji, Segoe UI Symbol, Noto Color Emoji';

export const FONT_FAMILY_PRESETS: { label: string; value: string }[] = [
	{ label: 'Poppins', value: DEFAULT_FONT_FAMILY },
	{
		label: 'Helvetica Neue',
		value:
			'Helvetica Neue, Helvetica, Arial, sans-serif, Apple Color Emoji, Segoe UI Emoji, Segoe UI Symbol, Noto Color Emoji'
	},
	{ label: 'System Default', value: 'ui-sans-serif, system-ui, sans-serif' }
];

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
			surface1Color: preferences?.theme?.surface1Color ?? 'hsl(0 0 calc(2.5 + 93))',
			surface2Color: preferences?.theme?.surface2Color ?? 'hsl(0 0 calc(2.5 + 90))',
			surface3Color: preferences?.theme?.surface3Color ?? 'hsl(0 0 calc(2.5 + 80))',
			secondaryColor: preferences?.theme?.secondaryColor ?? 'hsl(0 0 82.5)',
			successColor: preferences?.theme?.successColor ?? 'oklch(67% 0.13 149)',
			warningColor: preferences?.theme?.warningColor ?? 'oklch(79.5% 0.184 86.047)',
			errorColor: preferences?.theme?.errorColor ?? '#ef4444',
			primaryColor: preferences?.theme?.primaryColor ?? '#4f7ef3',
			onPrimaryColor: preferences?.theme?.onPrimaryColor ?? 'hsl(0 0 100)',
			onSuccessColor: preferences?.theme?.onSuccessColor ?? 'hsl(0 0 100)',
			onWarningColor: preferences?.theme?.onWarningColor ?? 'hsl(0 0 100)',
			onErrorColor: preferences?.theme?.onErrorColor ?? 'hsl(0 0 100)',
			darkBackgroundColor: preferences?.theme?.darkBackgroundColor ?? 'hsl(0 0 0)',
			darkOnBackgroundColor: preferences?.theme?.darkOnBackgroundColor ?? 'hsl(0 0 calc(2.5 + 95))',
			darkSurface1Color: preferences?.theme?.darkSurface1Color ?? 'hsl(0 0 calc(2.5 + 5))',
			darkSurface2Color: preferences?.theme?.darkSurface2Color ?? 'hsl(0 0 calc(2.5 + 10))',
			darkSurface3Color: preferences?.theme?.darkSurface3Color ?? 'hsl(0 0 calc(2.5 + 20))',
			darkSecondaryColor: preferences?.theme?.darkSecondaryColor ?? 'hsl(0 0 22.5)',
			darkSuccessColor: preferences?.theme?.darkSuccessColor ?? 'oklch(67% 0.13 149)',
			darkWarningColor: preferences?.theme?.darkWarningColor ?? 'oklch(79.5% 0.184 86.047)',
			darkErrorColor: preferences?.theme?.darkErrorColor ?? '#ef4444',
			darkPrimaryColor: preferences?.theme?.darkPrimaryColor ?? '#4f7ef3',
			darkOnPrimaryColor: preferences?.theme?.darkOnPrimaryColor ?? 'hsl(0 0 97.5)',
			darkOnSuccessColor: preferences?.theme?.darkOnSuccessColor ?? 'hsl(0 0 97.5)',
			darkOnWarningColor: preferences?.theme?.darkOnWarningColor ?? 'hsl(0 0 97.5)',
			darkOnErrorColor: preferences?.theme?.darkOnErrorColor ?? 'hsl(0 0 97.5)',
			fontFamily: preferences?.theme?.fontFamily ?? DEFAULT_FONT_FAMILY
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
	document.documentElement.style.setProperty('--theme-surface1-light', colors.surface1Color);
	document.documentElement.style.setProperty('--theme-surface2-light', colors.surface2Color);
	document.documentElement.style.setProperty('--theme-surface3-light', colors.surface3Color);
	document.documentElement.style.setProperty('--theme-primary-light', colors.primaryColor);
	document.documentElement.style.setProperty('--theme-on-primary-light', colors.onPrimaryColor);
	document.documentElement.style.setProperty('--theme-on-success-light', colors.onSuccessColor);
	document.documentElement.style.setProperty('--theme-on-warning-light', colors.onWarningColor);
	document.documentElement.style.setProperty('--theme-on-error-light', colors.onErrorColor);
	document.documentElement.style.setProperty('--theme-secondary-light', colors.secondaryColor);
	document.documentElement.style.setProperty('--theme-success-light', colors.successColor);
	document.documentElement.style.setProperty('--theme-warning-light', colors.warningColor);
	document.documentElement.style.setProperty('--theme-error-light', colors.errorColor);

	// Set dark theme colors
	document.documentElement.style.setProperty('--theme-background-dark', colors.darkBackgroundColor);
	document.documentElement.style.setProperty(
		'--theme-on-background-dark',
		colors.darkOnBackgroundColor
	);
	document.documentElement.style.setProperty('--theme-surface1-dark', colors.darkSurface1Color);
	document.documentElement.style.setProperty('--theme-surface2-dark', colors.darkSurface2Color);
	document.documentElement.style.setProperty('--theme-surface3-dark', colors.darkSurface3Color);
	document.documentElement.style.setProperty('--theme-primary-dark', colors.darkPrimaryColor);
	document.documentElement.style.setProperty('--theme-on-primary-dark', colors.darkOnPrimaryColor);
	document.documentElement.style.setProperty('--theme-on-success-dark', colors.darkOnSuccessColor);
	document.documentElement.style.setProperty('--theme-on-warning-dark', colors.darkOnWarningColor);
	document.documentElement.style.setProperty('--theme-on-error-dark', colors.darkOnErrorColor);
	document.documentElement.style.setProperty('--theme-secondary-dark', colors.darkSecondaryColor);
	document.documentElement.style.setProperty('--theme-success-dark', colors.darkSuccessColor);
	document.documentElement.style.setProperty('--theme-warning-dark', colors.darkWarningColor);
	document.documentElement.style.setProperty('--theme-error-dark', colors.darkErrorColor);
	document.documentElement.style.setProperty('--theme-font-family', colors.fontFamily);
}

function initialize(preferences: AppPreferences) {
	store.current = preferences;
	store.loaded = true;
	if (browser) {
		store.setThemeColors(store.current.theme);
	}
}

export default store;
