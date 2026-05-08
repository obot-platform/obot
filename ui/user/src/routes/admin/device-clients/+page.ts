import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { DeviceScanResponse, OrgUser } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';
import { compileDeviceClients } from './utils';

const PAGE_SIZE = 100;
export const load: PageLoad = async ({ fetch }) => {
	let devices: DeviceScanResponse = { items: [], total: 0, limit: PAGE_SIZE, offset: 0 };
	let users: OrgUser[] = [];
	try {
		[devices, users] = await Promise.all([
			AdminService.listDeviceScans({ limit: PAGE_SIZE, offset: 0, groupByDevice: true }, { fetch }),
			AdminService.listUsers({ fetch }).catch(() => [] as OrgUser[])
		]);
		const clientsMap = compileDeviceClients(devices.items ?? [], users ?? []);
		return { clients: [...clientsMap.values()] };
	} catch (err) {
		handleRouteError(err, '/admin/device-clients', profile.current);
	}
};
