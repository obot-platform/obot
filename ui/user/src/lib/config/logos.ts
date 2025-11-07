import { env } from '$env/dynamic/public';

// Logo configuration that can be customized via environment variables
// or by directly editing this file

// Default logo paths
const DEFAULT_LOGOS = {
	// Logo.svelte variants
	icon: {
		blue: '/user/images/obot-icon-blue.svg',
		white: '/user/images/obot-icon-white.svg',
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

// Logo configuration with environment variable overrides
export const LOGO_CONFIG = {
	icon: {
		blue: env.PUBLIC_LOGO_ICON_BLUE ?? DEFAULT_LOGOS.icon.blue,
		white: env.PUBLIC_LOGO_ICON_WHITE ?? DEFAULT_LOGOS.icon.white,
		error: env.PUBLIC_LOGO_ICON_ERROR ?? DEFAULT_LOGOS.icon.error,
		warning: env.PUBLIC_LOGO_ICON_WARNING ?? DEFAULT_LOGOS.icon.warning
	},
	beta: {
		dark: {
			chat: env.PUBLIC_LOGO_BETA_DARK_CHAT ?? DEFAULT_LOGOS.beta.dark.chat,
			enterprise: env.PUBLIC_LOGO_BETA_DARK_ENTERPRISE ?? DEFAULT_LOGOS.beta.dark.enterprise,
			default: env.PUBLIC_LOGO_BETA_DARK_DEFAULT ?? DEFAULT_LOGOS.beta.dark.default
		},
		light: {
			chat: env.PUBLIC_LOGO_BETA_LIGHT_CHAT ?? DEFAULT_LOGOS.beta.light.chat,
			enterprise: env.PUBLIC_LOGO_BETA_LIGHT_ENTERPRISE ?? DEFAULT_LOGOS.beta.light.enterprise,
			default: env.PUBLIC_LOGO_BETA_LIGHT_DEFAULT ?? DEFAULT_LOGOS.beta.light.default
		}
	}
} as const;

// Helper function to get logo path
export function getLogoPath(type: 'icon' | 'beta', variant: string, subVariant?: string): string {
	if (type === 'icon') {
		return LOGO_CONFIG.icon[variant as keyof typeof LOGO_CONFIG.icon] ?? DEFAULT_LOGOS.icon.blue;
	}

	if (type === 'beta' && subVariant) {
		const theme = variant as 'dark' | 'light';
		const style = subVariant as keyof typeof LOGO_CONFIG.beta.dark;
		return LOGO_CONFIG.beta[theme]?.[style] ?? DEFAULT_LOGOS.beta.light.default;
	}

	return DEFAULT_LOGOS.icon.blue;
}
