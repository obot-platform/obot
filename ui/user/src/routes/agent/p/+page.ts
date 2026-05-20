import { NanobotService } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const ssr = false;

export const load: PageLoad = async ({ fetch }) => {
	const projects = await NanobotService.listProjects({ fetch });
	if (projects.length === 0) {
		const project = await NanobotService.createProject({ displayName: 'New Project' }, { fetch });
		throw redirect(302, `/agent/p/${project.id}`);
	}
	throw redirect(302, `/agent/p/${projects[0].id}`);
};
