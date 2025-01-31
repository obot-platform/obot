import { EntityMeta } from "~/lib/model/primitives";

export type ProviderConfigurationParameter = {
	name: string;
	friendlyName?: string;
	description?: string;
	sensitive?: boolean;
};

export type ProviderStatus = {
	configured: boolean;
	icon?: string;
	iconNoInvert?: boolean;
	link?: string;
	description?: string;
	requiredConfigurationParameters?: ProviderConfigurationParameter[];
	optionalConfigurationParameters?: ProviderConfigurationParameter[];
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
