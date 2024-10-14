import { EntityMeta } from "~/lib/model/primitives";
import { Template } from "~/lib/model/workflows";

export type ToolReferenceBase = {
    name: string;
    toolType: ToolReferenceType;
    reference: string;
    metadata: Record<string, string>;
};

export type ToolReferenceType = "tool" | "stepTemplate";

export type ToolReference = {
    error: string;
    description: string;
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
