import { ToolReference } from "~/lib/model/toolReferences";

export const mockedDatabaseToolReference: ToolReference = {
	id: "database",
	created: "2025-01-29T11:10:12-05:00",
	revision: "1",
	metadata: {
		category: "Capability",
		icon: "https//www.mockimagelocation.com/database.svg",
	},
	type: "toolreference",
	name: "Database",
	toolType: "tool",
	reference: "github.com/obot-platform/tools/database",
	active: true,
	resolved: true,
	builtin: true,
	description: "Tools for interacting with a database",
};

export const mockedKnowledgeToolReference: ToolReference = {
	id: "knowledge",
	created: "2025-01-29T11:10:12-05:00",
	revision: "1",
	metadata: {
		category: "Capability",
		icon: "https//www.mockimagelocation.com/knowledge.svg",
		noUserAuth: "knowledge",
	},
	type: "toolreference",
	name: "Knowledge",
	toolType: "tool",
	reference: "github.com/obot-platform/tools/knowledge",
	active: true,
	resolved: true,
	builtin: true,
	description: "Obtain search result from the knowledge set",
	credentials: ["mock.com/credentials"],
	params: {
		Query: "A search query that will be evaluated against the knowledge set",
	},
};

export const mockedTasksToolReference: ToolReference = {
	id: "tasks",
	created: "2025-01-29T11:10:12-05:00",
	revision: "1",
	metadata: {
		category: "Capability",
		icon: "https//www.mockimagelocation.com/tasks.svg",
	},
	type: "toolreference",
	name: "Tasks",
	toolType: "tool",
	reference: "github.com/obot-platform/tools/tasks",
	active: true,
	resolved: true,
	builtin: true,
	description: "Manage and execute tasks",
};

export const mockedWorkspaceFilesToolReference: ToolReference = {
	id: "workspace-files",
	created: "2025-01-29T11:10:12-05:00",
	revision: "2695",
	metadata: {
		category: "Capability",
		icon: "https//www.mockimagelocation.com/workspacefiles.svg",
	},
	type: "toolreference",
	name: "Workspace Files",
	toolType: "tool",
	reference: "github.com/obot-platform/tools/workspace-files",
	active: true,
	resolved: true,
	builtin: true,
	description:
		"Adds the capability for users to read and write workspace files",
};

export const mockedToolReferences: ToolReference[] = [
	mockedDatabaseToolReference,
	mockedKnowledgeToolReference,
	mockedTasksToolReference,
	mockedWorkspaceFilesToolReference,
];
