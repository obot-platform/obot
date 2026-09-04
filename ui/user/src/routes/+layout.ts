import { dev } from '$app/environment';
import { getHttpStatusCode } from '$lib/errors';
import {
	AdminService,
	Group,
	UserService,
	type AppNotification,
	type AppPreferences,
	type DefaultModelAlias,
	type License,
	type Model,
	type ProductTelemetryConsent,
	type Profile,
	type Version
} from '$lib/services';
import { compileAppPreferences } from '$lib/stores/appPreferences.svelte';
import type { LayoutLoad } from './$types';

export const prerender = 'auto';
export const ssr = dev;

export const load: LayoutLoad = async ({ fetch }) => {
	const [versionResult, licenseResult, appPreferencesResult, profileResult] =
		await Promise.allSettled([
			UserService.getVersion({ fetch }),
			UserService.getLicense({ fetch }),
			UserService.listAppPreferences({ fetch }),
			UserService.getProfile({ fetch })
		]);

	const version: Version | undefined =
		versionResult.status === 'fulfilled' ? versionResult.value : undefined;
	const license: License | undefined =
		licenseResult.status === 'fulfilled' ? licenseResult.value : undefined;
	const appPreferences: AppPreferences =
		appPreferencesResult.status === 'fulfilled'
			? compileAppPreferences(appPreferencesResult.value)
			: compileAppPreferences();
	const profile: Profile =
		profileResult.status === 'fulfilled'
			? profileResult.value
			: {
					id: '',
					email: '',
					iconURL: '',
					role: 0,
					effectiveRole: 0,
					groups: [],
					unauthorized: true,
					username: ''
				};

	let defaultModelAliases: DefaultModelAlias[] | undefined;
	let models: Model[] | undefined;
	let appNotification: AppNotification | undefined;
	let productTelemetryConsent: ProductTelemetryConsent | undefined;
	let productTelemetryConsentAvailable: boolean | undefined;

	if (!profile.unauthorized) {
		const isAdmin = profile.groups.includes(Group.ADMIN);
		const [
			defaultModelAliasesResult,
			modelsResult,
			appNotificationResult,
			productTelemetryConsentResult
		] = await Promise.allSettled([
			UserService.listDefaultModelAliases({ fetch }),
			UserService.listModels({ fetch }),
			UserService.getAppNotification({ fetch }),
			isAdmin
				? AdminService.getProductTelemetryConsent({ fetch, dontLogErrors: true })
				: Promise.resolve(undefined)
		]);
		defaultModelAliases =
			defaultModelAliasesResult.status === 'fulfilled'
				? defaultModelAliasesResult.value
				: undefined;
		models = modelsResult.status === 'fulfilled' ? modelsResult.value : undefined;
		appNotification =
			appNotificationResult.status === 'fulfilled' ? appNotificationResult.value : undefined;

		if (isAdmin) {
			if (productTelemetryConsentResult.status === 'fulfilled') {
				productTelemetryConsent = productTelemetryConsentResult.value;
				productTelemetryConsentAvailable = true;
			} else {
				productTelemetryConsentAvailable =
					getHttpStatusCode(productTelemetryConsentResult.reason) === 404 ? false : undefined;
			}
		}
	}

	return {
		appPreferences,
		profile,
		version,
		license,
		defaultModelAliases,
		models,
		appNotification,
		productTelemetryConsent,
		productTelemetryConsentAvailable
	};
};
