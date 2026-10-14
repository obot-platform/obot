import { AdminService, UserService } from '$lib/services';
import type { VMCP } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();

	let vmcps: VMCP[];
	let workspaceId: string | undefined;

	if (!profile.isAdmin?.()) {
		try {
			workspaceId = await UserService.fetchWorkspaceIDForProfile(profile?.id, { fetch });
		} catch {
			workspaceId = undefined;
		}
	}

	try {
		vmcps = profile.hasAdminAccess?.()
			? await AdminService.listAllVMCPs({ fetch })
			: await UserService.listVMCPs({ fetch });
	} catch {
		vmcps = [];
	}

	return { vmcps, workspaceId };
};
