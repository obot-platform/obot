import { AdminService, UserService } from '$lib/services';
import type { VMCP } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();

	try {
		const vmcps = profile.hasAdminAccess?.()
			? await AdminService.listAllVMCPs({ fetch })
			: await UserService.listVMCPs({ fetch });
		return { vmcps };
	} catch {
		return { vmcps: [] as VMCP[] };
	}
};
