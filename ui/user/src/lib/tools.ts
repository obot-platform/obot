import type { AssistantTool } from '$lib/services';

export function hasTool(tools: AssistantTool[], tool: string): boolean {
	for (const t of tools) {
		if (t.id === tool) {
			return t.enabled || t.builtin || false;
		}
	}
	return false;
}
