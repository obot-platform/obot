import { Agent } from "~/lib/model/agents";

export const mockedAgent: Agent = {
	id: "a17m9ht",
	created: "2025-01-29T12:49:07-05:00",
	links: {
		invoke: "http://localhost:8080/api/invoke/a17m9ht",
	},
	type: "agent",
	name: "Grouchy Bacon",
	icons: null,
	description: "",
	temperature: null,
	cache: null,
	alias: "",
	prompt: "",
	knowledgeDescription: "",
	agents: null,
	workflows: null,
	tools: ["knowledge", "workspace-files", "database", "tasks"],
	availableThreadTools: null,
	defaultThreadTools: null,
	oauthApps: null,
	introductionMessage: "",
	starterMessages: null,
	params: null,
	model: "",
	env: null,
	aliasAssigned: false,
	toolInfo: {
		database: {
			authorized: true,
		},
		knowledge: {
			authorized: true,
		},
		tasks: {
			authorized: true,
		},
		"workspace-files": {
			authorized: true,
		},
	},
};
