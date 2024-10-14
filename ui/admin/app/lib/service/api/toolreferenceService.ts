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
    });

    return res.data.items ?? ([] as ToolReference[]);
}
getToolReferences.key = (type?: ToolReferenceType) =>
    ({
        url: ApiRoutes.toolReferences.base({ type }).path,
    }) as const;

export type ToolCategory = {
    bundleTool?: ToolReference;
    tools: ToolReference[];
};
export type ToolCategoryMap = Record<string, ToolCategory>;
async function getToolReferencesCategoryMap(type?: ToolReferenceType) {
    const res = await request<{ items: ToolReference[] }>({
        url: ApiRoutes.toolReferences.base({ type }).url,
    });

    const toolReferences = res.data.items;
    const result: ToolCategoryMap = {};

    for (const toolReference of toolReferences) {
        const category = toolReference.metadata?.category || "Uncategorized";

        if (!result[category]) {
            result[category] = {
                tools: [],
            };
        }

        if (toolReference.metadata?.bundle) {
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
    }) as const;

const getToolReferenceById = async (toolReferenceId: string) => {
    const res = await request<ToolReference>({
        url: ApiRoutes.toolReferences.getById(toolReferenceId).url,
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
    });

    return res.data;
}

async function deleteToolReference(id: string) {
    await request({
        url: ApiRoutes.toolReferences.getById(id).url,
        method: "DELETE",
    });
}

const revalidateToolReferences = () =>
    revalidateWhere((url) =>
        url.includes(ApiRoutes.toolReferences.base().path)
    );

export const ToolReferenceService = {
    getToolReferences,
    getToolReferencesCategoryMap,
    getToolReferenceById,
    createToolReference,
    updateToolReference,
    deleteToolReference,
    revalidateToolReferences,
};
