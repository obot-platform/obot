import { RunState } from "@gptscript-ai/gptscript";

import { AgentIcons } from "~/lib/model/agents";
import { EntityMeta } from "~/lib/model/primitives";

export type ThreadManifest = {
	name?: string;
	description?: string;
	tools?: string[];
	icons?: Nullish<AgentIcons>;
	revision?: string;
	prompt: string;
	knowledgeDescription: string;
	introductionMessage: string;
	starterMessages: Nullish<string[]>;
};

export type Thread = EntityMeta &
	ThreadManifest & {
		state?: RunState;
		currentRunId?: string;
		projectID?: string;
		lastRunID?: string;
		userID?: string;
		project?: boolean;
	} & (
		| { agentID: string; workflowID?: never }
		| { agentID?: never; workflowID: string }
	);

export type UpdateThread = ThreadManifest;
