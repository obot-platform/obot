import { ChatService, NanobotService } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const ssr = false;

export const load: PageLoad = async ({ fetch, params, parent }) => {
	const { profile } = await parent();
	const version = await ChatService.getVersion({ fetch });
	if (!version.nanobotIntegration) {
		throw redirect(302, '/');
	}

	const workflowName = params.workflowName;
	const projectId = params.projectId;
	const publishedWorkflows = await NanobotService.listPublishedWorkflows({ fetch });
	const publishedInfo = publishedWorkflows.find(
		(w) => w.name === workflowName && w.authorID === profile.id
	);

	return {
		workflowName,
		projectId,
		publishedInfo
	};
};
