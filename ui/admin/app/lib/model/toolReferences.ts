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
	error: string;
	description: string;
	builtin: boolean;
	params: Record<string, string>;
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
export type ToolCategoryMap = Record<string, ToolCategory>;

export function convertToolReferencesToCategoryMap(
	toolReferences: ToolReference[]
) {
	const result: ToolCategoryMap = {};

	for (const toolReference of toolReferences) {
		if (toolReference.deleted) {
			// skip tools if marked with deleted
			continue;
		}

		const category = !toolReference.builtin
			? CustomToolsToolCategory
			: toolReference.metadata?.category || UncategorizedToolCategory;

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
