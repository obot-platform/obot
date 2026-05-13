import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { DeviceClientFleetSummary, OrgUser } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	let client: DeviceClientFleetSummary = {
		name: '',
		users: [],
		skills: [],
		mcpServers: []
	};
	let users: OrgUser[] = [];
	try {
		[client, users] = await Promise.all([
			AdminService.getDeviceClient(params.name, { fetch }),
			AdminService.listUsers({ fetch }).catch(() => [] as OrgUser[])
		]);
		return { client, users };
	} catch (err) {
		handleRouteError(err, `/admin/device-clients/${params.name}`, profile.current);
	}
};
