import { AgentIcons } from "~/lib/model/agents";
import { EntityMeta } from "~/lib/model/primitives";

export const AssistantNamespace = {
    Agents: "agents",
    Workflows: "workflows",
} as const;
export type AssistantNamespace =
    (typeof AssistantNamespace)[keyof typeof AssistantNamespace];

export const AssistantType = {
    Agent: "agent",
    Workflow: "workflow",
} as const;
export type AssistantType = (typeof AssistantType)[keyof typeof AssistantType];

export type Assistant = EntityMeta & {
    name: string;
    entityID: string;
    description: string;
    icons: AgentIcons;
    type: AssistantType;
};
