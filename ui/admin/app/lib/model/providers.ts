import { EntityMeta } from "~/lib/model/primitives";

export type ProviderStatus = {
	configured: boolean;
	icon?: string;
	requiredConfigurationParameters?: string[];
	optionalConfigurationParameters?: string[];
	missingConfigurationParameters?: string[];
};

export type Provider = EntityMeta &
	ProviderStatus & {
		toolReference: string;
		name: string;
		revision: string;
	};

export type ProviderConfig = Record<string, string>;

export type ModelProvider = Provider & {
	modelsBackPopulated?: boolean;
};

export type AuthProvider = Provider;
