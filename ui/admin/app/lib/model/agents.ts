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
    params?: Record<string, string>;
};

export type Agent = EntityMeta &
    AgentBase & {
        slugAssigned: boolean;
    };

export type CreateAgent = AgentBase;
export type UpdateAgent = AgentBase;
