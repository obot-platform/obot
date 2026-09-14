import { AdminService, UserService } from '$lib/services';
import type { VMCP } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch, parent, url }) => {
	const { profile } = await parent();

	let vmcps: VMCP[];
	try {
		vmcps = profile.hasAdminAccess?.()
			? await AdminService.listAllVMCPs({ fetch })
			: await UserService.listVMCPs({ fetch });
	} catch {
		vmcps = [];
	}

	if (vmcps.length === 0 && profile.hasAdminAccess?.() && !url.searchParams.has('new')) {
		throw redirect(307, `${url.pathname}?new=true`);
	}

	return { vmcps };
};
