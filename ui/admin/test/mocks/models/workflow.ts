import { Workflow } from "~/lib/model/workflows";

export const mockedWorkflow: Workflow = {
	id: "w1dshmx",
	created: "2025-01-30T17:03:06-05:00",
	links: {
		invoke: "http://localhost:8080/api/invoke/w1dshmx",
	},
	type: "workflow",
	name: "Giving Turmeric",
	icons: null,
	description: "",
	temperature: null,
	cache: null,
	alias: "",
	prompt: "",
	knowledgeDescription: "",
	agents: null,
	workflows: null,
	tools: ["knowledge", "workspace-files", "database"],
	availableThreadTools: null,
	defaultThreadTools: null,
	oauthApps: null,
	introductionMessage: "",
	starterMessages: null,
	params: null,
	model: "",
	env: null,
	steps: null,
	output: "",
	aliasAssigned: false,
	toolInfo: {
		database: {
			authorized: true,
		},
		knowledge: {
			authorized: true,
		},
		"workspace-files": {
			authorized: true,
		},
	},
};
