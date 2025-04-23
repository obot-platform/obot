import { ChatService, EditorService, type Project } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ url, fetch }) => {
	const mcpId = url.searchParams.get('id');
	let project: Project;
	try {
		project = await EditorService.createObot({ fetch });

		if (mcpId) {
			await ChatService.configureProjectMCP(project.assistantID, project.id, mcpId, { fetch });

			// TEMP: add the mcpID as a bundle to the project tools
			const tools = (await ChatService.listTools(project.assistantID, project.id, { fetch })).items;
			const matchingIndex = tools.findIndex((tool) => tool.id === mcpId);
			if (matchingIndex !== -1) {
				tools[matchingIndex].enabled = true;
				await ChatService.updateProjectTools(project.assistantID, project.id, {
					items: tools
				});
			}
		}

		throw redirect(303, `/o/${project.id}`);
	} catch (err) {
		if (!(err instanceof Error)) {
			throw err;
		}

		// unauthorized, go home
		if (err.message?.includes('401') || err.message?.includes('unauthorized')) {
			throw redirect(303, `/`);
		}

		// otherwise, let error throw
		throw err;
	}
};
