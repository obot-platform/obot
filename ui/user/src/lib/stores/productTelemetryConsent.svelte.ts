import type { ProductTelemetryConsent } from '$lib/services';

export const PRODUCT_ANALYTICS_DISMISSED_KEY = '@obot/product-analytics-consent-dismissed';

const store = $state<{
	available: boolean | undefined;
	consent: boolean | undefined;
	initialize: (value?: ProductTelemetryConsent, available?: boolean) => void;
	setConsent: (consent: boolean) => void;
}>({
	available: undefined,
	consent: undefined,
	initialize,
	setConsent
});

function initialize(value?: ProductTelemetryConsent, available?: boolean) {
	store.available = available;
	store.consent = value?.consent;
}

function setConsent(consent: boolean) {
	store.available = true;
	store.consent = consent;
}

export function dismissProductAnalyticsPrompt() {
	if (typeof sessionStorage !== 'undefined') {
		sessionStorage.setItem(PRODUCT_ANALYTICS_DISMISSED_KEY, 'true');
	}
}

export function clearProductAnalyticsPromptDismissal() {
	if (typeof sessionStorage !== 'undefined') {
		sessionStorage.removeItem(PRODUCT_ANALYTICS_DISMISSED_KEY);
	}
}

export function isProductAnalyticsPromptDismissed() {
	return (
		typeof sessionStorage !== 'undefined' &&
		sessionStorage.getItem(PRODUCT_ANALYTICS_DISMISSED_KEY) === 'true'
	);
}

export default store;
