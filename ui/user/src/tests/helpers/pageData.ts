import { Group } from '$lib/services';
import type { Profile } from '$lib/services/user/types';
import {
	accessibleModels,
	appNotification,
	appPreferences,
	defaultModelAliases,
	license as licenseStore,
	profile,
	userDeviceSettings,
	version
} from '$lib/stores';
import { compileAppPreferences } from '$lib/stores/appPreferences.svelte';
import type { LayoutData } from '../../routes/$types';
import {
	getAppNotificationResponse,
	getLicenseResponse,
	getProfileResponse,
	getVersionResponse,
	listAppPreferencesResponse,
	listDefaultModelAliasesResponse,
	listModelsResponse
} from '../mocks/data';

type PageDataOverrides = Partial<LayoutData> & Record<string, unknown>;

/**
 * Builds a Profile with the role helper methods the UI expects at runtime.
 */
export function createMockProfile(groups: string[] = [Group.ADMIN]): Profile {
	return {
		...getProfileResponse,
		groups,
		iconURL: '',
		loaded: true,
		canImpersonate: () => groups.includes(Group.ADMIN) && groups.includes(Group.USER_IMPERSONATION),
		hasAdminAccess: () => groups.includes(Group.ADMIN) || groups.includes(Group.AUDITOR),
		isAdmin: () => groups.includes(Group.ADMIN),
		isAdminReadonly: () => !groups.includes(Group.ADMIN) && groups.includes(Group.AUDITOR),
		isBootstrapUser: () => false
	};
}

/**
 * Creates root layout `data` (from `src/routes/+layout.ts`) with defaults from MSW mocks.
 * Pass overrides for any layout field, or additional page-specific load fields.
 *
 * @example
 * ```ts
 * const data = createPageData<PageData>({ license: mockLicense });
 * render(LicensePage, { data });
 * ```
 */
export function createPageData<T = LayoutData>(overrides: PageDataOverrides = {}): T {
	return {
		appPreferences: compileAppPreferences(listAppPreferencesResponse),
		profile: createMockProfile(),
		version: getVersionResponse,
		license: getLicenseResponse,
		defaultModelAliases: listDefaultModelAliasesResponse,
		models: listModelsResponse,
		appNotification: getAppNotificationResponse,
		...overrides
	} as T;
}

/**
 * Initializes client stores the same way `src/routes/+layout.svelte` does from layout data.
 */
export async function initializePageStores(data: LayoutData) {
	userDeviceSettings.setShowAllGuides(false);

	if (data.appPreferences) {
		appPreferences.initialize(data.appPreferences);
	}
	if (data.profile) {
		profile.initialize(data.profile);
	}
	if (data.version) {
		version.initialize(data.version);
	}
	if (data.appNotification) {
		await appNotification.initialize(data.appNotification);
	}

	licenseStore.initialize(data.license);

	if (data.defaultModelAliases) {
		await defaultModelAliases.initialize(data.defaultModelAliases);
	}
	if (data.models) {
		await accessibleModels.initialize(data.models);
	}
}

/**
 * Convenience helper for route `page.svelte.spec.ts` files: builds layout/page data and
 * initializes stores so components that rely on either `data` or stores work in tests.
 *
 * @example
 * ```ts
 * const data = await preparePageData<PageData>({
 *   license: { ...getLicenseResponse, licenseKey: 'key' },
 *   version: { ...getVersionResponse, userCount: 90, userLimit: 100 }
 * });
 * render(LicensePage, { data });
 * ```
 */
export async function preparePageData<T = LayoutData>(
	overrides: PageDataOverrides = {}
): Promise<T> {
	const data = createPageData<T & LayoutData>(overrides);
	await initializePageStores(data);
	return data;
}
