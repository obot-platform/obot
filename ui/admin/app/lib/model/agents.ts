import { EnvVariable } from "~/lib/model/environmentVariables";
import { EntityMeta } from "~/lib/model/primitives";
import { User } from "~/lib/model/users";

// TODO: implement as zod schemas???

export const KNOWLEDGE_TOOL = "knowledge";

export type AgentBase = {
	name: string;
	description: string;
	temperature?: number | null;
	cache?: boolean | null;
	alias: string;
	aliasAssigned?: boolean;
	prompt: string;
	agents?: string[] | null;
	workflows?: string[] | null;
	tools?: string[];
	defaultThreadTools?: string[] | null;
	availableThreadTools?: Nullish<string[]>;
	params?: Record<string, string> | null;
	knowledgeDescription?: string;
	model?: string;
	toolInfo?: AgentToolInfo;
	env?: EnvVariable[] | null;
	starterMessages?: string[] | null;
	introductionMessage?: string;
	icons: AgentIcons | null;
	oauthApps?: string[] | null;
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
		default?: boolean;
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

export type AgentAuthorization = {
	userID: string;
	agentId: string;
	user?: User;
};
