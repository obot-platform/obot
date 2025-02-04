import { RunState } from "@gptscript-ai/gptscript";

import { EntityMeta } from "~/lib/model/primitives";

export type ThreadBase = {
	description?: string;
	tools?: string[];
};

export type Thread = EntityMeta &
	ThreadBase & {
		state?: RunState;
		currentRunId?: string;
		parentThreadId?: string;
		lastRunID?: string;
		userID?: string;
	} & (
		| { agentID: string; workflowID?: never }
		| { agentID?: never; workflowID: string }
	);

export type UpdateThread = ThreadBase;
