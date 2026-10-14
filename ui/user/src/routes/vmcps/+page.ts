import { AdminService, UserService } from '$lib/services';
import type { VMCP } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();

	let vmcps: VMCP[];
	let workspaceId: string | undefined;
	try {
		vmcps = profile.hasAdminAccess?.()
			? await AdminService.listAllVMCPs({ fetch })
			: await UserService.listVMCPs({ fetch });

		if (!profile.isAdmin?.()) {
			workspaceId = await UserService.fetchWorkspaceIDForProfile(profile?.id, { fetch });
		}
	} catch {
		vmcps = [];
	}

	return { vmcps, workspaceId };
};
