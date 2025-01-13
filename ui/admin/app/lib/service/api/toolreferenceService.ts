import {
	CreateToolReference,
	ToolReference,
	ToolReferenceType,
	UpdateToolReference,
} from "~/lib/model/toolReferences";
import { ApiRoutes, revalidateWhere } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

async function getToolReferences(type?: ToolReferenceType) {
	const res = await request<{ items: ToolReference[] }>({
		url: ApiRoutes.toolReferences.base({ type }).url,
		errorMessage: "Failed to fetch tool references",
	});

	return res.data.items ?? ([] as ToolReference[]);
}
getToolReferences.key = (type?: ToolReferenceType) =>
	({
		url: ApiRoutes.toolReferences.base({ type }).path,
		type,
	}) as const;
getToolReferences.revalidate = (type?: ToolReferenceType) => {
	revalidateWhere((url) =>
		url.includes(ApiRoutes.toolReferences.base({ type }).path)
	);
};

export type ToolCategory = {
	bundleTool?: ToolReference;
	tools: ToolReference[];
};
export const UncategorizedToolCategory = "Uncategorized";
export const CustomToolsToolCategory = "Custom Tools";
export type ToolCategoryMap = Record<string, ToolCategory>;
async function getToolReferencesCategoryMap(type?: ToolReferenceType) {
	const res = await request<{ items: ToolReference[] }>({
		url: ApiRoutes.toolReferences.base({ type }).url,
		errorMessage: "Failed to fetch tool references category map",
	});

	const toolReferences = res.data.items;
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
getToolReferencesCategoryMap.key = (type?: ToolReferenceType) =>
	({
		url: ApiRoutes.toolReferences.base({ type }).path,
		responseType: "map",
	}) as const;

const getToolReferenceById = async (toolReferenceId: string) => {
	const res = await request<ToolReference>({
		url: ApiRoutes.toolReferences.getById(toolReferenceId).url,
		errorMessage: "Failed to fetch tool reference",
	});

	return res.data;
};
getToolReferenceById.key = (toolReferenceId?: Nullish<string>) => {
	if (!toolReferenceId) return null;

	return {
		url: ApiRoutes.toolReferences.getById(toolReferenceId).path,
		toolReferenceId,
	};
};

async function createToolReference({
	toolReference,
}: {
	toolReference: CreateToolReference;
}) {
	const res = await request<ToolReference>({
		url: ApiRoutes.toolReferences.base().url,
		method: "POST",
		data: toolReference,
		errorMessage: "Failed to create tool reference",
	});

	return res.data;
}

async function updateToolReference({
	id,
	toolReference,
}: {
	id: string;
	toolReference: UpdateToolReference;
}) {
	const res = await request<ToolReference>({
		url: ApiRoutes.toolReferences.getById(id).url,
		method: "PUT",
		data: toolReference,
		errorMessage: "Failed to update tool reference",
	});

	return res.data;
}

async function forceRefreshToolReference(id: string) {
	const res = await request<ToolReference>({
		url: ApiRoutes.toolReferences.purgeCache(id).url,
		method: "POST",
		errorMessage: "Failed to force refresh tool reference",
	});

	return res.data;
}

async function deleteToolReference(id: string) {
	await request({
		url: ApiRoutes.toolReferences.getById(id).url,
		method: "DELETE",
		errorMessage: "Failed to delete tool reference",
	});
}

export const ToolReferenceService = {
	getToolReferences,
	getToolReferencesCategoryMap,
	getToolReferenceById,
	createToolReference,
	updateToolReference,
	deleteToolReference,
	forceRefreshToolReference,
};
