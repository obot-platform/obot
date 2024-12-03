import { EntityMeta } from "~/lib/model/primitives";

export type ModelProviderStatus = {
    configured: boolean;
    requiredConfigurationParameters?: string[];
    missingConfigurationParameters?: string[];
};

export type ModelProvider = EntityMeta &
    ModelProviderStatus & {
        toolReference: string;
        name: string;
        revision: string;
    };

export type ModelProviderConfig = Record<string, string>;
