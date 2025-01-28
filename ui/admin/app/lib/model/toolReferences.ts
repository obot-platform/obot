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
	description: string;
	builtin: boolean;
	active: boolean;
	credentials?: string[];
	error?: string;
	params?: Record<string, string>;
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

export type ToolCategory = {
	bundleTool?: ToolReference;
	tools: ToolReference[];
};
export const UncategorizedToolCategory = "Uncategorized";
export const CustomToolsToolCategory = "Custom Tools";
export const CapabilitiesToolCategory = "Capability";

export type ToolCategoryMap = Record<string, ToolCategory>;

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

export function convertToolReferencesToCategoryMap(
	toolReferences: ToolReference[]
) {
	const result: ToolCategoryMap = {};

	for (const toolReference of toolReferences) {
		if (toolReference.deleted) {
			// skip tools if marked with deleted
			continue;
		}

		// skip capabilities
		if (isCapabilityTool(toolReference)) {
			continue;
		}

		const category =
			toolReference.metadata?.category || UncategorizedToolCategory;

		if (!result[category]) {
			result[category] = {
				tools: [],
			};
		}

		if (toolReference.metadata?.bundle === "true") {
			result[category].bundleTool = toolReference;
		} else {
			result[category].tools.push(toolReference);
		}
	}

	return result;
}
