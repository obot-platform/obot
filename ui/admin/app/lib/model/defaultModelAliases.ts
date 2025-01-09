import { ModelAlias } from "~/lib/model/models";

export type DefaultModelAliasBase = {
	alias: ModelAlias;
	model: string;
};

export type DefaultModelAlias = DefaultModelAliasBase;

export type CreateDefaultModelAlias = DefaultModelAliasBase;
export type UpdateDefaultModelAlias = DefaultModelAliasBase;
