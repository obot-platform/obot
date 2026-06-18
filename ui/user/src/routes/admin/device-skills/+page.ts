import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { DeviceSkillSortKey, DeviceSkillStatResponse } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';
import { DEFAULT_SORT_BY, DEFAULT_SORT_ORDER, PAGE_SIZE, sortFields } from './constants';

function getSortBy(property: string | null): DeviceSkillSortKey {
	const key = property ?? '';
	return Object.hasOwn(sortFields, key) ? sortFields[key] : DEFAULT_SORT_BY;
}

function getSortOrder(order: string | null): 'asc' | 'desc' {
	return order === 'asc' ? 'asc' : 'desc';
}

export const load: PageLoad = async ({ url, fetch }) => {
	const offset = parseInt(url.searchParams.get('offset') ?? '0', 10) || 0;
	const name = url.searchParams.get('name') ?? '';
	const sortProperty = url.searchParams.get('sort');
	const hasValidSortProperty = sortProperty != null && Object.hasOwn(sortFields, sortProperty);
	const sortBy = getSortBy(sortProperty);
	const sortOrder = hasValidSortProperty
		? getSortOrder(url.searchParams.get('sortDirection'))
		: DEFAULT_SORT_ORDER;

	let skills: DeviceSkillStatResponse;
	try {
		skills = await AdminService.listDeviceSkills(
			{ limit: PAGE_SIZE, offset, name: name || undefined, sortBy, sortOrder },
			{ fetch }
		);
		return { skills, pageSize: PAGE_SIZE };
	} catch (err) {
		handleRouteError(err, '/admin/device-skills', profile.current);
	}
};
