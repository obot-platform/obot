import { EnvVariable } from "~/lib/model/environmentVariables";
import { EntityMeta } from "~/lib/model/primitives";

// TODO: implement as zod schemas???

export const KNOWLEDGE_TOOL = "knowledge";

export type AgentBase = {
	name: string;
	description: string;
	temperature?: number;
	cache?: boolean;
	alias: string;
	aliasAssigned?: boolean;
	prompt: string;
	agents?: string[];
	workflows?: string[];
	tools?: string[];
	defaultThreadTools?: string[];
	availableThreadTools?: Nullish<string[]>;
	params?: Record<string, string>;
	knowledgeDescription?: string;
	model?: string;
	toolInfo?: AgentToolInfo;
	env?: EnvVariable[];
};

export type AgentOAuthStatus = {
	url?: string;
	authenticated?: boolean;
	required?: boolean | null;
	error?: string;
};

export type Agent = EntityMeta &
	AgentBase & {
		authStatus?: Record<string, AgentOAuthStatus>;
	};

export type CreateAgent = AgentBase;
export type UpdateAgent = AgentBase;

export type AgentIcons = {
	icon: string;
	iconDark: string;
	collapsed: string;
	collapsedDark: string;
};

export type ToolInfo = {
	credentialNames?: string[];
	authorized: boolean;
};

export type AgentToolInfo = Record<string, ToolInfo>;

export type Authorization = {
	userID: string;
	agentId: string;
};
