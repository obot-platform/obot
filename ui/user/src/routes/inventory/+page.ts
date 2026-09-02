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

const views = new Set([
	'overview',
	'configuration',
	'devices',
	'device-clients',
	'device-mcp-servers',
	'device-skills'
]);

export const load: PageLoad = async ({ url, fetch, parent, depends }) => {
	// Named so pages that rewrite this fleet's policy — the enforcement decisions
	// page adds allowlist entries of its own — can mark this data stale.
	depends('devices:data');

	const { profile } = await parent();
	const requestedView = url.searchParams.get('view');
	let view = requestedView && views.has(requestedView) ? requestedView : undefined;
	const end = url.searchParams.get('end') ?? new Date().toISOString();
	const start =
		url.searchParams.get('start') ?? new Date(Date.now() - DEFAULT_WINDOW_MS).toISOString();

	let stats: DeviceScanStats | null = null;
	let configuration: MDMConfiguration | undefined;
	let enrollmentKeys: MDMEnrollmentKey[] = [];
	let assetSource: MDMAssetSource | undefined;
	let assets: MDMAsset[] = [];
	let assetLoadError: string | undefined;

	if (!profile.hasAdminAccess?.()) {
		return {
			stats,
			range: { start, end },
			configuration,
			enrollmentKeys,
			assetSource,
			assets,
			assetLoadError
		};
	}

	async function loadConfigurations() {
		try {
			const configurations = await AdminService.listMDMConfigurations({ fetch });
			configuration = configurations.find((candidate) => candidate.isDefault) ?? configurations[0];
		} catch (err) {
			handleRouteError(err, '/inventory', profile);
		}
	}

	async function loadStats() {
		try {
			stats = await AdminService.getDeviceScanStats({ start, end }, { fetch });
		} catch (err) {
			handleRouteError(err, '/inventory', profile);
		}
	}

	async function loadConfigurationDetails() {
		const [sourceResult, assetsResult] = await Promise.allSettled([
			AdminService.getMDMAssetSource({ fetch }),
			AdminService.listMDMAssets({ fetch })
		]);

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
				handleRouteError(err, '/inventory?view=configuration', profile);
			}
		}
	}

	if (!view) {
		await loadConfigurations();
		view = configuration ? 'overview' : 'configuration';
	}

	switch (view) {
		case 'overview':
			await loadStats();
			break;
		case 'configuration':
			if (!configuration) {
				await loadConfigurations();
			}
			await loadConfigurationDetails();
			break;
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
