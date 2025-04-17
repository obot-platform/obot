export type ToolInput = {
	internalToolName: string;
	content: string;
};

export type KnowledgeToolOutput = {
	url?: string;
	content: string;
}[];

export type GoogleSearchOutput = {
	duration: { search: number; refine: number; response: number };
	query: string;
	results: { url: string; content: string }[];
};

export type TavilySearchOutput = {
	query: string;
	follow_up_questions: string;
	answer: string;
	images: string[];
	results: {
		title: string;
		url: string;
		content: string;
		score: number;
		raw_content: string;
	}[];
};

export type ToolCall = {
	name: string;
	description: string;
	input: string;
	output: string;
	metadata?: {
		category?: string;
		icon?: string;
		toolBundle?: string;
	};
};

type PromptAuthMeta = {
	authURL?: string;
	category?: string;
	icon?: string;
	toolContext?: string;
	toolDisplayName?: string;
	authType: "oauth" | "basic";
};

export type AuthPrompt = {
	id?: string;
	name: string;
	time?: Date;
	message: string;
	fields?: PromptField[];
	sensitive?: boolean;
	metadata?: PromptAuthMeta;
};

export type PromptField = {
	options?: string[];
	name: string;
	description?: string;
	sensitive?: boolean;
};

// note(ryanhopperlowe) renaming this to ChatEvent to differentiate itself specifically for a chat with an agent
// we should create a separate type for WorkflowEvents and leverage Unions to differentiate between them
export type ChatEvent = {
	content: string;
	input?: string;
	contentID?: string;
	replayComplete?: boolean;
	error?: string;
	runComplete?: boolean;
	runID: string;
	waitingOnModel?: boolean;
	toolInput?: ToolInput;
	toolCall?: ToolCall;
	prompt?: AuthPrompt;
	time?: string;
};

export function combineChatEvents(events: ChatEvent[]): ChatEvent[] {
	const combinedEvents: ChatEvent[] = [];

	let buildingEvent: ChatEvent | null = null;

	const insertBuildingEvent = () => {
		if (buildingEvent) {
			combinedEvents.push(buildingEvent);
			buildingEvent = null;
		}
	};

	for (const event of events) {
		const { content, input, error, runID, toolCall, prompt, contentID } = event;

		// signals the end of a content block
		if (error || toolCall || input || prompt) {
			insertBuildingEvent();

			combinedEvents.push(event);
			continue;
		}

		if (content) {
			if (!buildingEvent) {
				buildingEvent = {
					content: "",
					runID,
					contentID,
				};
			}

			buildingEvent.content += content;
		}
	}

	insertBuildingEvent();

	return combinedEvents;
}
