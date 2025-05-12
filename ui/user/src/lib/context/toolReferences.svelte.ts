import type { ToolReference } from '$lib/services';
import { getContext, hasContext, setContext } from 'svelte';

const Key = Symbol('toolReferences');

type ToolReferenceContext = {
	items: ToolReference[];
};

export function initToolReferences(toolReferences: ToolReference[]) {
	const data = $state<ToolReferenceContext>({ items: toolReferences });
	setContext(Key, data);
}

export function getToolReferences() {
	if (!hasContext(Key)) {
		throw new Error('Tool references not initialized');
	}
	return getContext<ToolReferenceContext>(Key).items;
}

export function getToolReferenceMap() {
	const toolReferences = getToolReferences();
	return new Map(toolReferences.map((x) => [x.id, x]));
}

type ToolBundleItem = {
	tool: ToolReference;
	bundleTools: ToolReference[];
};

export function getToolBundleMap() {
	const map = new Map<string, ToolBundleItem>();

	const toolReferences = getToolReferences();

	// init all bundle tools
	const topLevelTools = toolReferences.filter((x) => x.bundle);
	for (const tool of topLevelTools) {
		map.set(tool.id, { tool, bundleTools: [] });
	}

	for (const toolReference of toolReferences) {
		// skip bundle tools
		if (toolReference.bundle) {
			continue;
		}

		const { bundleToolName } = toolReference;

		// for singlular tools with no bundled subtools, we need to add them to the map
		if (!bundleToolName) {
			map.set(toolReference.id, { tool: toolReference, bundleTools: [] });
			continue;
		}

		const current = map.get(bundleToolName) ?? {
			tool: toolReference,
			bundleTools: []
		};

		current.bundleTools.push(toolReference);
		map.set(bundleToolName, current);
	}

	return map;
}
