import { handleRouteError } from '$lib/errors';
import { UserService, type OrgUser, type DeviceScanResponse } from '$lib/services';
import type { PageLoad } from './$types';

const PAGE_SIZE = 50;

export const load: PageLoad = async ({ url, fetch, parent }) => {
	const { profile } = await parent();
	const hasAdminAccess = profile.hasAdminAccess?.();
	const offset = parseInt(url.searchParams.get('offset') ?? '0', 10) || 0;

	let devices: DeviceScanResponse;
	let users: OrgUser[] = [];
	try {
		if (hasAdminAccess) {
			[devices, users] = await Promise.all([
				UserService.listDeviceScans({ limit: PAGE_SIZE, offset, groupByDevice: true }, { fetch }),
				UserService.listUsers({ fetch }).catch(() => [] as OrgUser[])
			]);
		} else {
			devices = await UserService.listDeviceScans(
				{ limit: PAGE_SIZE, offset, groupByDevice: true },
				{ fetch }
			);
		}
		return { devices, users, pageSize: PAGE_SIZE };
	} catch (err) {
		const prefix = profile.hasAdminAccess?.() ? '/admin' : '';
		handleRouteError(err, `${prefix}/devices`, profile);
	}
};
