import { AdminService, UserService } from '$lib/services';
import type { VMCP } from '$lib/services';
import type { GitCredential, VMcpRepository } from '$lib/services/admin/types';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

const views = new Set(['vmcps', 'sources']);

export const load: PageLoad = async ({ fetch, parent, url }) => {
	const { profile } = await parent();
	if (!profile.isAdmin?.()) {
		throw redirect(307, '/');
	}

	const requestedView = url.searchParams.get('view');
	const view = requestedView && views.has(requestedView) ? requestedView : 'vmcps';

	let vmcps: VMCP[] = [];
	let vmcpRepositories: VMcpRepository[] = [];
	let gitCredentials: GitCredential[] = [];

	if (view === 'vmcps') {
		try {
			vmcps = await UserService.listVMCPs({ fetch });
		} catch {
			vmcps = [];
		}
	}

	if (view === 'sources') {
		try {
			[vmcpRepositories, gitCredentials] = await Promise.all([
				AdminService.listVMcpRepositories({ fetch, dontLogErrors: true }),
				AdminService.listGitCredentials({ fetch, dontLogErrors: true }).catch(() => [])
			]);
		} catch {
			vmcpRepositories = [];
		}
	}

	return {
		vmcps,
		vmcpRepositories,
		gitCredentials
	};
};
