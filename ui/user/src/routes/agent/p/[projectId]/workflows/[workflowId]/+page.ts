import { ChatService, NanobotService } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const ssr = false;

export const load: PageLoad = async ({ fetch, params }) => {
	const version = await ChatService.getVersion({ fetch });
	if (!version.nanobotIntegration) {
		throw redirect(302, '/');
	}

	const workflowId = params.workflowId;
	const projectId = params.projectId;
	const publishedWorkflows = await NanobotService.listPublishedWorkflows({ fetch });

	return {
		workflowId,
		projectId,
		publishedWorkflows
	};
};
