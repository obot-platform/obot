import { NanobotService } from '$lib/services';
import type { PageLoad } from './$types';

export const ssr = false;

export const load: PageLoad = async ({ fetch, params }) => {
	const workflowId = params.workflowId;
	const projectId = params.projectId;
	const publishedWorkflows = await NanobotService.listPublishedWorkflows({ fetch });

	return {
		workflowId,
		projectId,
		publishedWorkflows
	};
};
