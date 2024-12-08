import { EntityMeta } from "~/lib/model/primitives";

export type ModelProviderStatus = {
    configured: boolean;
    modelsBackPopulated?: boolean;
    icon?: string;
    requiredConfigurationParameters?: string[];
    missingConfigurationParameters?: string[];
    sensitiveConfigurationParameters?: string[];
};

export type ModelProvider = EntityMeta &
    ModelProviderStatus & {
        toolReference: string;
        name: string;
        revision: string;
    };

export type ModelProviderConfig = Record<string, string>;
