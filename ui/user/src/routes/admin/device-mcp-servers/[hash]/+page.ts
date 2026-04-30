import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type {
	DeviceMCPServerDetail,
	DeviceMCPServerOccurrenceList
} from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

const PAGE_SIZE = 50;

export const load: PageLoad = async ({
	params,
	url,
	fetch
}: {
	params: { hash: string };
	url: URL;
	fetch: typeof globalThis.fetch;
}) => {
	const offset = parseInt(url.searchParams.get('offset') ?? '0', 10) || 0;
	let detail: DeviceMCPServerDetail | null = null;
	let occurrences: DeviceMCPServerOccurrenceList = {
		items: [],
		total: 0,
		limit: PAGE_SIZE,
		offset: 0
	};
	try {
		[detail, occurrences] = await Promise.all([
			AdminService.getDeviceMCPServerDetail(params.hash, { fetch }),
			AdminService.listDeviceMCPServerOccurrences(
				params.hash,
				{ limit: PAGE_SIZE, offset },
				{ fetch }
			)
		]);
		return { detail, occurrences, pageSize: PAGE_SIZE };
	} catch (err) {
		handleRouteError(err, `/admin/device-mcp-servers/${params.hash}`, profile.current);
	}
};
