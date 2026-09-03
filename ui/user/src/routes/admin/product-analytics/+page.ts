import { handleRouteError } from '$lib/errors';
import { AdminService, Group, type ProductTelemetryConsent } from '$lib/services';
import type { PageLoad } from './$types';
import { error } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch, parent }) => {
	const data = await parent();

	if (!data.profile.groups.includes(Group.ADMIN)) {
		throw error(403, { message: 'Product Analytics settings require administrator access.' });
	}

	if (data.productTelemetryConsentAvailable === false) {
		throw error(404, { message: 'Product Analytics consent controls are unavailable.' });
	}

	if (data.productTelemetryConsentAvailable === true && data.productTelemetryConsent) {
		return { productTelemetryConsent: data.productTelemetryConsent };
	}

	try {
		const productTelemetryConsent: ProductTelemetryConsent =
			await AdminService.getProductTelemetryConsent({ fetch, dontLogErrors: true });
		return { productTelemetryConsent };
	} catch (err) {
		handleRouteError(err, '/admin/product-analytics', data.profile);
	}
};
