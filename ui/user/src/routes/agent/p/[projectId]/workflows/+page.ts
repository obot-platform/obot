import { NanobotService } from '$lib/services';
import type { PageLoad } from './$types';

export const ssr = false;

export const load: PageLoad = async ({ fetch }) => {
	const publishedWorkflows = await NanobotService.listPublishedWorkflows({ fetch });
	return { publishedWorkflows };
};
