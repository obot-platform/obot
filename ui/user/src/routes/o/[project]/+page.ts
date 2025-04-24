import { browser } from '$app/environment';
import { handleRouteError } from '$lib/errors';
import { ChatService } from '$lib/services';
import type { AssistantTool, ToolReference } from '$lib/services/chat/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

/**
 * Enhances the tools array with capability information from toolReferences
 */
function enhanceToolsWithCapabilities(
	tools: AssistantTool[],
	toolReferences: ToolReference[]
): AssistantTool[] {
	// Create a map of tool references by ID for faster lookup
	const toolReferenceMap = new Map<string, ToolReference>();
	for (const reference of toolReferences) {
		toolReferenceMap.set(reference.id, reference);
	}

	// Enhance each tool with capability information
	return tools.map((tool) => {
		const toolRef = toolReferenceMap.get(tool.id);
		return {
			...tool,
			capability: toolRef?.metadata?.category === 'Capability'
		};
	});
}

export const load: PageLoad = async ({ params, fetch }) => {
	try {
		const [project, toolReferences] = await Promise.all([
			ChatService.getProject(params.project, { fetch }),
			ChatService.listAllTools({ fetch })
		]);

		const [tools, assistant] = await Promise.all([
			ChatService.listTools(project.assistantID, project.id, { fetch }),
			ChatService.getAssistant(project.assistantID, { fetch })
		]);

		if (browser) {
			localStorage.setItem('lastVisitedObot', params.project);
		}

		// Enhance tools with capability information
		const enhancedTools = enhanceToolsWithCapabilities(tools.items, toolReferences.items);

		return {
			project,
			tools: enhancedTools,
			toolReferences: toolReferences.items,
			assistant
		};
	} catch (e) {
		handleRouteError(e, `/o/${params.project}`, profile.current);
	}
};
