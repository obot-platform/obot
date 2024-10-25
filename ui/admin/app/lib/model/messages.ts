import { ChatEvent, OAuthPrompt, ToolCall } from "./chatEvents";
import { Run } from "./runs";

export interface Message {
    text: string;
    sender: "user" | "agent";
    // note(ryanhopperlowe) we only support one tool call per message for now
    // leaving it as an array case that changes in the future
    prompt?: OAuthPrompt;
    tools?: ToolCall[];
    runId?: string;
    isLoading?: boolean;
    error?: boolean;
}

export const runsToMessages = (runs: Run[]) => {
    const messages = [] as Message[];
    for (const run of runs) {
        messages.push({
            text: run.input,
            sender: "user",
            runId: run.id,
        });

        if (run.output) {
            messages.push({
                text: run.output,
                sender: "agent",
                runId: run.id,
            });
        }
    }
    return messages;
};

export const toolCallMessage = (toolCall: ToolCall): Message => ({
    sender: "agent",
    text: `Tool call: ${[toolCall.metadata?.category, toolCall.name].filter((x) => !!x).join(" - ")}`,
    tools: [toolCall],
});

export const promptMessage = (prompt: OAuthPrompt, runID: string): Message => ({
    sender: "agent",
    text: prompt.message,
    prompt,
    runId: runID,
});

export const chatEventsToMessages = (events: ChatEvent[]) => {
    const messages: Message[] = [];

    for (const event of events) {
        const { content, input, toolCall, runID, error, prompt } = event;

        if (error) {
            messages.push({
                sender: "agent",
                text: `Error: ${error}`,
                runId: runID,
                error: true,
            });
            continue;
        }

        if (input) {
            messages.push({
                sender: "user",
                text: input,
                runId: runID,
            });
            continue;
        }

        if (toolCall) {
            messages.push(toolCallMessage(toolCall));
            continue;
        }

        if (prompt) {
            messages.push(promptMessage(prompt, runID));
            continue;
        }

        if (content) {
            messages.push({
                sender: "agent",
                text: content,
                runId: runID,
            });
        }
    }

    return messages;
};
