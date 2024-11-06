import { EntityMeta } from "~/lib/model/primitives";

// TODO: implement as zod schemas???

export type AgentBase = {
    name: string;
    description: string;
    temperature?: number;
    cache?: boolean;
    refName: string;
    prompt: string;
    agents?: string[];
    workflows?: string[];
    tools?: string[];
    defaultThreadTools?: string[];
    availableThreadTools?: string[];
    params?: Record<string, string>;
    knowledgeDescription?: string;
};

export type AgentOAuthStatus = {
    url?: string;
    authenticated?: boolean;
    required?: boolean | null;
    error?: string;
};

export type Agent = EntityMeta &
    AgentBase & {
        slugAssigned: boolean;
    } & {
        authStatus?: Record<string, AgentOAuthStatus>;
    };

export type CreateAgent = AgentBase;
export type UpdateAgent = AgentBase;
