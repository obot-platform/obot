import type { AssistantTool } from '$lib/services';

const store = $state({
	current: {
		tools: [] as AssistantTool[],
		maxTools: Number.MAX_SAFE_INTEGER
	},
	setMaxTools: (maxTools: number) => {
		store.current.maxTools = maxTools;
	},
	setTools: (tools: AssistantTool[]) => {
		store.current.tools = tools;
	}
});

export default store;
