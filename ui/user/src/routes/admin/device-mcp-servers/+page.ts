import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { DeviceScanStats } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ url, fetch }) => {
	const start = url.searchParams.get('start') ?? undefined;
	const end = url.searchParams.get('end') ?? undefined;

	let stats: DeviceScanStats = {
		timeStart: '',
		timeEnd: '',
		deviceCount: 0,
		userCount: 0,
		clients: [],
		mcpServers: [],
		skills: [],
		scanTimestamps: []
	};
	try {
		stats = await AdminService.getDeviceScanStats({ start, end }, { fetch });
		return { stats };
	} catch (err) {
		handleRouteError(err, '/admin/device-mcp-servers', profile.current);
	}
};
