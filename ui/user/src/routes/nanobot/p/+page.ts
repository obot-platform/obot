import { ChatService, NanobotService } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const ssr = false;

export const load: PageLoad = async ({ fetch, url }) => {
	const version = await ChatService.getVersion({ fetch });
	if (!version.nanobotIntegration) {
		throw redirect(302, '/');
	}

	const planner = url.searchParams.get('planner') === 'true' ? '?planner=true' : '';

	const projects = await NanobotService.listProjectsV2({ fetch });
	if (projects.length === 0) {
		const project = await NanobotService.createProjectV2({ displayName: 'New Project' }, { fetch });
		throw redirect(302, `/nanobot/p/${project.id}${planner}`);
	}
	throw redirect(302, `/nanobot/p/${projects[0].id}${planner}`);
};
