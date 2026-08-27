import { handleRouteError } from '$lib/errors';
import { UserService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	let workspace;
	const [parentData, workspacesResult] = await Promise.all([
		parent(),
		UserService.listWorkspaces({ fetch })
			.then((workspaces) => ({ workspaces }))
			.catch((error: unknown) => ({ error }))
	]);
	const { profile } = parentData;

	if ('workspaces' in workspacesResult) {
		workspace = workspacesResult.workspaces.find((w) => w.userID === profile?.id) ?? null;
	} else {
		handleRouteError(workspacesResult.error, `/mcp-servers`, profile);
	}

	return {
		workspace
	};
};
