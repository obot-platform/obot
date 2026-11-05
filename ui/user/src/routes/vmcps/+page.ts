import { UserService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();

	let workspaceId: string | undefined;

	if (!profile.isAdmin?.()) {
		try {
			workspaceId = await UserService.fetchWorkspaceIDForProfile(profile?.id, { fetch });
		} catch {
			workspaceId = undefined;
		}
	}

	return { workspaceId };
};
