import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { DeviceSkillStatResponse } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

const PAGE_SIZE = 50;

export const load: PageLoad = async ({ url, fetch }) => {
	const offset = parseInt(url.searchParams.get('offset') ?? '0', 10) || 0;
	const name = url.searchParams.get('name') ?? '';

	let skills: DeviceSkillStatResponse = { items: [], total: 0, limit: PAGE_SIZE, offset };
	try {
		skills = await AdminService.listDeviceSkills(
			{ limit: PAGE_SIZE, offset, name: name || undefined },
			{ fetch }
		);
		return { skills, pageSize: PAGE_SIZE };
	} catch (err) {
		handleRouteError(err, '/admin/device-skills', profile.current);
	}
};
