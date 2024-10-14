import { AgentBase } from "~/lib/model/agents";
import { EntityMeta } from "~/lib/model/primitives";

export type WorkflowBase = AgentBase & {
    steps: Step[];
    output: string;
};

export type Step = {
    id: string;
    name: string;
    description: string;
    if?: If;
    while?: While;
    template?: Template;
    step?: string;
    cache: boolean;
    temperature: number;
    tools: string[];
    agents: string[];
    workflows: string[];
};

export type Template = {
    name: string;
    args: Record<string, string>;
};

export type Subflow = {
    workflow: string;
};

export type If = {
    condition: string;
    steps: Step[];
    else: Step[];
};

export type While = {
    condition: string;
    maxLoops: number;
    steps: Step[];
};

export type Workflow = EntityMeta &
    WorkflowBase & {
        slugAssigned: boolean;
    };

export type CreateWorkflow = WorkflowBase;
export type UpdateWorkflow = WorkflowBase;
