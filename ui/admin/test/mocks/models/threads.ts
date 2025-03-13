import { RunState } from "@gptscript-ai/gptscript";
import { mockedAgent } from "test/mocks/models/agents";
import { mockedUser } from "test/mocks/models/users";

import { Thread } from "~/lib/model/threads";

export const mockedThreads: Thread[] = [
	{
		id: "t1bm2kn",
		created: "2025-02-04T10:53:11-05:00",
		type: "thread",
		assistantID: mockedAgent.id,
		state: RunState.Continue,
		lastRunID: "r1g9hw7",
		userID: mockedUser.id,
		introductionMessage: "Hello, how are you?",
		knowledgeDescription: "You have knowledge about the world.",
		starterMessages: ["Hello, how are you?"],
		name: "My Thread",
		prompt: "You are a helpful assistant.",
	},
];
