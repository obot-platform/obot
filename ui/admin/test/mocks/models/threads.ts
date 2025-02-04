import { RunState } from "@gptscript-ai/gptscript";
import { mockedAgent } from "test/mocks/models/agents";
import { mockedUser } from "test/mocks/models/users";

import { Thread } from "~/lib/model/threads";

export const mockedThreads: Thread[] = [
	{
		id: "t1bm2kn",
		created: "2025-02-04T10:53:11-05:00",
		type: "thread",
		agentID: mockedAgent.id,
		state: RunState.Continue,
		lastRunID: "r1g9hw7",
		userID: mockedUser.id,
	},
];
