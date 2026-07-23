import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type {
	DeviceScanStats,
	MDMAsset,
	MDMAssetSource,
	MDMConfiguration,
	MDMEnrollmentKey
} from '$lib/services/admin/types';
import type { PageLoad } from './$types';
import { DEFAULT_WINDOW_MS } from './constants';

export const load: PageLoad = async ({ url, fetch, parent }) => {
	const { profile } = await parent();
	const end = url.searchParams.get('end') ?? new Date().toISOString();
	const start =
		url.searchParams.get('start') ?? new Date(Date.now() - DEFAULT_WINDOW_MS).toISOString();

	let stats: DeviceScanStats | null = null;
	let configuration: MDMConfiguration | undefined;
	let enrollmentKeys: MDMEnrollmentKey[] = [];
	let assetSource: MDMAssetSource | undefined;
	let assets: MDMAsset[] = [];
	let assetLoadError: string | undefined;

	const [statsResult, configurationsResult, sourceResult, assetsResult] = await Promise.allSettled([
		AdminService.getDeviceScanStats({ start, end }, { fetch }),
		AdminService.listMDMConfigurations({ fetch }),
		AdminService.getMDMAssetSource({ fetch }),
		AdminService.listMDMAssets({ fetch })
	]);

	if (statsResult.status === 'fulfilled') {
		stats = statsResult.value;
	} else {
		handleRouteError(statsResult.reason, '/admin/devices', profile);
	}

	if (configurationsResult.status === 'fulfilled') {
		configuration =
			configurationsResult.value.find((candidate) => candidate.isDefault) ??
			configurationsResult.value[0];
	} else {
		handleRouteError(configurationsResult.reason, '/admin/devices', profile);
	}

	if (sourceResult.status === 'fulfilled') {
		assetSource = sourceResult.value;
	} else {
		assetLoadError = 'Unable to load the MDM asset source.';
	}

	if (assetsResult.status === 'fulfilled') {
		assets = assetsResult.value;
	} else {
		assetLoadError ??= 'Unable to load MDM assets.';
	}

	if (configuration) {
		try {
			enrollmentKeys = await AdminService.listMDMEnrollmentKeys(configuration.id, { fetch });
		} catch (err) {
			handleRouteError(err, '/admin/devices?view=configuration', profile);
		}
	}

	return {
		stats,
		range: { start, end },
		configuration,
		enrollmentKeys,
		assetSource,
		assets,
		assetLoadError
	};
};
