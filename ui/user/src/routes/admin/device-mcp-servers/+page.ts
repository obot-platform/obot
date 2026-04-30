import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { DeviceScanStats } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ url, fetch }) => {
	const start = url.searchParams.get('start') ?? undefined;
	const end = url.searchParams.get('end') ?? undefined;

	let stats: DeviceScanStats = {
		time_start: '',
		time_end: '',
		device_count: 0,
		user_count: 0,
		clients: [],
		mcp_servers: [],
		skills: [],
		scan_timestamps: []
	};
	try {
		stats = await AdminService.getDeviceScanStats({ start, end }, { fetch });
		return { stats };
	} catch (err) {
		handleRouteError(err, '/admin/device-mcp-servers', profile.current);
	}
};
