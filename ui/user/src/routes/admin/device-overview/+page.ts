import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { DeviceScanStats } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';
import { DEFAULT_WINDOW_MS } from './constants';

export const load: PageLoad = async ({ url, fetch }) => {
	const end = url.searchParams.get('end') ?? new Date().toISOString();
	const start =
		url.searchParams.get('start') ?? new Date(Date.now() - DEFAULT_WINDOW_MS).toISOString();

	let stats: DeviceScanStats | null = null;
	try {
		stats = await AdminService.getDeviceScanStats({ start, end }, { fetch });
		return { stats, range: { start, end } };
	} catch (err) {
		handleRouteError(err, '/admin/device-overview', profile.current);
	}
};
