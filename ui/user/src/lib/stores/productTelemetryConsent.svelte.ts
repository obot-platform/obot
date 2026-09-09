import type { ProductTelemetryConsent } from '$lib/services';

export const PRODUCT_ANALYTICS_DEFERRED_KEY = '@obot/product-analytics-consent-deferred';

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

export function deferProductAnalyticsConsent() {
	if (typeof sessionStorage !== 'undefined') {
		sessionStorage.setItem(PRODUCT_ANALYTICS_DEFERRED_KEY, 'true');
	}
}

export function clearProductAnalyticsConsentDeferral() {
	if (typeof sessionStorage !== 'undefined') {
		sessionStorage.removeItem(PRODUCT_ANALYTICS_DEFERRED_KEY);
	}
}

export function isProductAnalyticsConsentDeferred() {
	return (
		typeof sessionStorage !== 'undefined' &&
		sessionStorage.getItem(PRODUCT_ANALYTICS_DEFERRED_KEY) === 'true'
	);
}

export default store;
