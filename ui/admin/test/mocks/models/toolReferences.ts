import { ToolReference } from "~/lib/model/toolReferences";

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
	bundle: false,
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
	bundle: false,
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
	bundle: false,
};

export const mockedImageToolBundle: ToolReference[] = [
	{
		id: "images-bundle",
		created: "2025-01-29T11:10:12-05:00",
		revision: "1",
		metadata: {
			bundle: "true",
			icon: "https://www.mock.com/assets/images_icon.svg",
		},
		type: "toolreference",
		name: "Images",
		toolType: "tool",
		reference: "github.com/obot-platform/tools/images",
		active: true,
		resolved: true,
		builtin: true,
		description: "Tools for analyzing and generating images",
		credentials: ["github.com/gptscript-ai/credentials/model-provider"],
		bundle: true,
	},
	{
		id: "images-analyze-images",
		created: "2025-01-29T11:10:12-05:00",
		revision: "1",
		metadata: {
			icon: "https://www.mock.com/assets/images_icon.svg",
			noUserAuth: "sys.model.provider.credential",
		},
		type: "toolreference",
		name: "Analyze Images",
		toolType: "tool",
		reference: "Analyze Images from github.com/obot-platform/tools/images",
		active: true,
		resolved: true,
		builtin: true,
		description:
			"Analyze images using a given prompt and return relevant information about the images",
		credentials: ["github.com/gptscript-ai/credentials/model-provider"],
		params: {
			images:
				"(required) A JSON array containing one or more URLs or file paths of images to analyze. Only supports jpeg, png, and webp.",
			prompt:
				'(optional) A prompt to analyze the images with (defaults "Provide a brief description of each image")',
		},
		bundle: false,
	},
];

export const mockedBrowserToolBundle: ToolReference[] = [
	{
		id: "browser-bundle",
		created: "2025-01-29T11:10:12-05:00",
		revision: "1",
		metadata: {
			bundle: "true",
			icon: "https://www.mock.com/assets/browser_icon.svg",
			noUserAuth: "sys.model.provider.credential",
		},
		type: "toolreference",
		name: "Browser",
		toolType: "tool",
		reference: "github.com/obot-platform/tools/browser",
		active: true,
		resolved: true,
		builtin: true,
		description: "Tools to navigate websites using a browser.",
		credentials: ["github.com/gptscript-ai/credentials/model-provider"],
		bundle: true,
	},
	{
		id: "browser-download-file-from-url",
		created: "2025-01-29T11:10:12-05:00",
		revision: "1",
		metadata: {
			icon: "https://www.mock.com/assets/browser_icon.svg",
		},
		type: "toolreference",
		name: "Download File From URL",
		toolType: "tool",
		reference:
			"Download File From URL from github.com/obot-platform/tools/browser",
		active: true,
		resolved: true,
		builtin: true,
		description:
			"Downloads a binary file from an HTTP/HTTPS URL and saves it to the workspace.",
		params: {
			fileName:
				"(required) The name of the workspace file to save the content to.",
			url: "(required) The URL of the file to download.",
		},
		bundle: false,
		bundleToolName: "browser-bundle",
	},
];

export const mockedBundleWithOauthReference: ToolReference[] = [
	{
		id: "google-gmail-bundle",
		created: "2025-02-05T13:54:26-05:00",
		revision: "1",
		metadata: {
			bundle: "true",
			icon: "gmail_icon_small.png",
			oauth: "google",
		},
		type: "toolreference",
		name: "Gmail",
		toolType: "tool",
		reference: "github.com/obot-platform/tools/google/gmail",
		active: true,
		resolved: true,
		builtin: true,
		description: "Tools for interacting with a user's Gmail account",
		credentials: ["github.com/obot-platform/tools/google/credential"],
		bundle: true,
	},
	{
		id: "google-gmail-list-drafts",
		created: "2025-02-05T13:54:26-05:00",
		revision: "1",
		metadata: {
			icon: "gmail_icon_small.png",
			oauth: "google",
		},
		type: "toolreference",
		name: "List Drafts",
		toolType: "tool",
		reference: "List Drafts from github.com/obot-platform/tools/google/gmail",
		active: true,
		resolved: true,
		builtin: true,
		description: "List drafts in a user's Gmail account",
		credentials: ["github.com/obot-platform/tools/google/credential"],
		params: {
			attachments:
				"A comma separated list of workspace file paths to attach to the email (Optional)",
			max_results:
				"Maximum number of drafts to list (Optional: Default will list 100 drafts)",
		},
		bundle: false,
		bundleToolName: "google-gmail-bundle",
	},
];

export const mockedToolReferences: ToolReference[] = [
	mockedKnowledgeToolReference,
	mockedTasksToolReference,
	mockedWorkspaceFilesToolReference,
	...mockedImageToolBundle,
	...mockedBrowserToolBundle,
	...mockedBundleWithOauthReference,
];
