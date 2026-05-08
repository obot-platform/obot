import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { DeviceScanResponse, OrgUser } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import { compileDeviceClients } from '../utils';
import type { PageLoad } from './$types';

const PAGE_SIZE = 100;

export const load: PageLoad = async ({ params, fetch }) => {
	let devices: DeviceScanResponse = { items: [], total: 0, limit: PAGE_SIZE, offset: 0 };
	let users: OrgUser[] = [];
	try {
		[devices, users] = await Promise.all([
			AdminService.listDeviceScans({ limit: PAGE_SIZE, offset: 0, groupByDevice: true }, { fetch }),
			AdminService.listUsers({ fetch }).catch(() => [] as OrgUser[])
		]);
		const clients = compileDeviceClients(devices.items ?? [], users ?? []);
		return { client: clients.get(params.name) };
	} catch (err) {
		handleRouteError(err, `/admin/device-clients/${name}`, profile.current);
	}
};
