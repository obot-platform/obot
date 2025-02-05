import { EntityMeta } from "~/lib/model/primitives";
import { Template } from "~/lib/model/workflows";

export type ToolReferenceBase = {
	name: string;
	toolType: ToolReferenceType;
	reference: string;
	resolved?: boolean;
	metadata?: Record<string, string>;
	revision: string;
};

export type ToolReferenceType = "tool" | "stepTemplate" | "modelProvider";

export type ToolReference = {
	bundle: boolean;
	bundleToolName?: string;
	tools?: ToolReference[];
	description: string;
	builtin: boolean;
	active: boolean;
	credentials?: string[];
	error?: string;
	params?: Record<string, string>;
	commit?: string;
} & EntityMeta &
	ToolReferenceBase;

export type CreateToolReference = ToolReferenceBase;
export type UpdateToolReference = ToolReferenceBase;

export const toolReferenceToTemplate = (toolReference: ToolReference) => {
	return {
		name: toolReference.id,
		args: toolReference.params,
	} as Template;
};

export const UncategorizedToolCategory = "Uncategorized";
export const CustomToolsToolCategory = "Custom Tools";
export const CapabilitiesToolCategory = "Capability";

export type ToolMap = Record<string, ToolReference>;

export const CapabilityTool = {
	Knowledge: "knowledge",
	WorkspaceFiles: "workspace-files",
	Database: "database",
	Tasks: "tasks",
} as const;
export type CapabilityTool =
	(typeof CapabilityTool)[keyof typeof CapabilityTool];

export function isCapabilityTool(toolReference: ToolReference) {
	return toolReference.metadata?.category === CapabilitiesToolCategory;
}

export function convertToolReferencesToMap(toolReferences: ToolReference[]) {
	// Convert array of tools to a map keyed by tool name
	const toolMap = new Map(toolReferences.map((tool) => [tool.id, tool]));
	const result: ToolMap = {};

	for (const toolReference of toolReferences) {
		if (toolReference.deleted || isCapabilityTool(toolReference)) {
			continue;
		}

		if (toolReference.bundle) {
			// Handle bundle tool
			if (!result[toolReference.id]) {
				result[toolReference.id] = {
					...toolReference,
					tools: [],
				};
			}
		} else if (toolReference.bundleToolName) {
			// Handle tool that belongs to a bundle
			const bundleTool = toolMap.get(toolReference.bundleToolName);
			if (bundleTool && !isCapabilityTool(bundleTool)) {
				if (!result[toolReference.bundleToolName]) {
					result[toolReference.bundleToolName] = {
						...bundleTool,
						tools: [toolReference],
					};
				} else {
					if (!result[toolReference.bundleToolName].tools) {
						throw new Error("This should never happen");
					}
					result[toolReference.bundleToolName].tools?.push(toolReference);
				}
			}
		} else {
			// Handle standalone tool
			result[toolReference.id] = {
				...toolReference,
				tools: [],
			};
		}
	}

	return result;
}
