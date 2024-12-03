import { ModelUsage } from "~/lib/model/models";
import { EntityMeta } from "~/lib/model/primitives";

export type AvailableModel = EntityMeta<{ usage?: ModelUsage }> & {
    object: string;
    owned_by: string;
    permission: string[];
    root: string;
    parent: string;
};
