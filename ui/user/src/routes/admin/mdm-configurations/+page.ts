import { handleRouteError } from '$lib/errors';
import {
	AdminService,
	type MDMAsset,
	type MDMAssetSource,
	type MDMConfiguration
} from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	let configurations: MDMConfiguration[] = [];
	let assetSource: MDMAssetSource | undefined;
	let assets: MDMAsset[] = [];
	let assetLoadError: string | undefined;

	const [configurationsResult, sourceResult, assetsResult] = await Promise.allSettled([
		AdminService.listMDMConfigurations({ fetch }),
		AdminService.getMDMAssetSource({ fetch }),
		AdminService.listMDMAssets({ fetch })
	]);
	if (configurationsResult.status === 'fulfilled') {
		configurations = configurationsResult.value;
	} else {
		handleRouteError(configurationsResult.reason, '/admin/mdm-configurations', profile);
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

	return { configurations, assetSource, assets, assetLoadError };
};
