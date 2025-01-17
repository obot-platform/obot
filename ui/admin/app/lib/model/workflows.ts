import { AgentBase } from "~/lib/model/agents";
import { EntityMeta } from "~/lib/model/primitives";

export type WorkflowBase = AgentBase & {
	steps: Step[];
	output: string;
};

export const StepType = {
	Command: "command",
	If: "if",
	While: "while",
	// Template: "template",
} as const;
export type StepType = (typeof StepType)[keyof typeof StepType];

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
	agents?: string[];
	workflows?: string[];
};

export type Template = {
	name: string;
	args: Record<string, string>;
};

export type Subflow = {
	workflow: string;
};

export type If = {
	condition?: string;
	steps?: Step[];
	else?: Step[];
};

export type While = {
	condition?: string;
	maxLoops?: number;
	steps?: Step[];
};

export type Workflow = EntityMeta &
	WorkflowBase & {
		slugAssigned: boolean;
	};

export type CreateWorkflow = Partial<WorkflowBase> & Pick<WorkflowBase, "name">;
export type UpdateWorkflow = WorkflowBase;

export const getDefaultStep = (
	type: StepType = StepType.Command,
	id?: string
): Step => {
	const newStep: Step = {
		id: id || crypto.randomUUID(),
		name: "",
		description: "",
		step: "",
		cache: false,
		temperature: 0,
		tools: [],
		agents: [],
		workflows: [],
	};

	if (type === StepType.If) {
		newStep.if = { condition: "", steps: [], else: [] };
	} else if (type === StepType.While) {
		newStep.while = { condition: "", maxLoops: 0, steps: [] };
	}

	return newStep;
};
