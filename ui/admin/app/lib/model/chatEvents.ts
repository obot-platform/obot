export type ToolInput = {
    internalToolName: string;
    content: string;
};

export type ToolCall = {
    name: string;
    description: string;
    input: string;
    metadata?: {
        category?: string;
        icon?: string;
    };
};

type PromptOAuthMeta = {
    authType: string;
    authURL: string;
    toolContext: string;
    toolDisplayName: string;
};

export type Prompt = {
    id?: string;
    time?: Date;
    message?: string;
    fields?: string[];
    sensitive?: boolean;
    metadata?: PromptOAuthMeta;
};

// note(ryanhopperlowe) renaming this to ChatEvent to differentiate itself specifically for a chat with an agent
// we should create a separate type for WorkflowEvents and leverage Unions to differentiate between them
export type ChatEvent = {
    content: string;
    input?: string;
    contentID?: string;
    error?: string;
    runID: string;
    waitingOnModel?: boolean;
    toolInput?: ToolInput;
    toolCall?: ToolCall;
    prompt?: Prompt;
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
        const { content, input, error, runID, toolCall, prompt } = event;

        // signals the end of a content block
        if (error || toolCall || input || prompt) {
            insertBuildingEvent();

            combinedEvents.push(event);
            continue;
        }

        if (content) {
            if (!buildingEvent) {
                buildingEvent = { content: "", runID };
            }

            buildingEvent.content += content;
        }
    }

    insertBuildingEvent();

    return combinedEvents;
}
