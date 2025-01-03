import {
    ReactNode,
    createContext,
    useCallback,
    useContext,
    useEffect,
    useState,
} from "react";
import { mutate } from "swr";

import { ChatEvent } from "~/lib/model/chatEvents";
import { Message, promptMessage, toolCallMessage } from "~/lib/model/messages";
import { InvokeService } from "~/lib/service/api/invokeService";
import { ThreadsService } from "~/lib/service/api/threadsService";

import { useAsync } from "~/hooks/useAsync";

type Mode = "agent" | "workflow";

interface ChatContextType {
    messages: Message[];
    mode: Mode;
    processUserMessage: (text: string) => void;
    abortRunningThread: () => void;
    id: string;
    threadId: Nullish<string>;
    invoke: (prompt?: string) => void;
    readOnly?: boolean;
    isRunning: boolean;
    isInvoking: boolean;
}

const ChatContext = createContext<ChatContextType | undefined>(undefined);

export function ChatProvider({
    children,
    id,
    mode = "agent",
    threadId,
    onCreateThreadId,
    readOnly,
    onRunEvent,
}: {
    children: ReactNode;
    mode?: Mode;
    id: string;
    threadId?: Nullish<string>;
    onCreateThreadId?: (threadId: string) => void;
    readOnly?: boolean;
    /** @description THIS MUST BE MEMOIZED */
    onRunEvent?: (event: ChatEvent) => void;
}) {
    const invoke = (prompt?: string) => {
        if (readOnly) return;

        if (mode === "workflow") invokeAgent.execute({ slug: id, prompt });
        else if (mode === "agent")
            invokeAgent.execute({ slug: id, prompt, thread: threadId });
    };

    const invokeAgent = useAsync(InvokeService.invokeAgentWithStream, {
        onSuccess: ({ threadId: responseThreadId }) => {
            if (responseThreadId && responseThreadId !== threadId) {
                // persist the threadId
                onCreateThreadId?.(responseThreadId);

                // revalidate threads
                mutate(ThreadsService.getThreads.key());
            }
        },
    });

    const { messages, isRunning } = useMessageSource(threadId, onRunEvent);

    const abortRunningThread = () => {
        if (!threadId || !isRunning) return;
        abortThreadProcess.execute(threadId);
    };

    const abortThreadProcess = useAsync(ThreadsService.abortThread);

    return (
        <ChatContext.Provider
            value={{
                messages,
                processUserMessage: invoke,
                abortRunningThread,
                mode,
                id,
                threadId,
                invoke,
                isRunning,
                isInvoking: invokeAgent.isLoading,
                readOnly,
            }}
        >
            {children}
        </ChatContext.Provider>
    );
}

export function useChat() {
    const context = useContext(ChatContext);
    if (context === undefined) {
        throw new Error("useChat must be used within a ChatProvider");
    }
    return context;
}

function useMessageSource(
    threadId?: Nullish<string>,
    onRunEvent?: (event: ChatEvent) => void
) {
    const [messages, setMessages] = useState<Message[]>([]);
    const [isRunning, setIsRunning] = useState(false);

    const addContent = useCallback(
        (event: ChatEvent) => {
            const {
                content,
                prompt,
                toolCall,
                runComplete,
                input,
                error,
                runID,
                contentID,
                replayComplete,
            } = event;

            onRunEvent?.(event);

            setIsRunning(!runComplete && !replayComplete);

            setMessages((prev) => {
                const copy = [...prev];

                // todo(ryanhopperlowe) can be optmized by searching from the end
                const existingIndex = contentID
                    ? copy.findIndex((m) => m.contentID === contentID)
                    : -1;

                if (existingIndex !== -1) {
                    const existing = copy[existingIndex];
                    copy[existingIndex] = {
                        ...existing,
                        text: existing.text + content,
                    };

                    return copy;
                }

                if (error) {
                    if (error.includes("thread was aborted, cancelling run")) {
                        copy.push({
                            sender: "agent",
                            text: "Message Aborted",
                            runId: runID,
                            contentID,
                            aborted: true,
                        });

                        return copy;
                    }

                    copy.push({
                        sender: "agent",
                        text: error,
                        runId: runID,
                        error: true,
                        contentID,
                    });
                    return copy;
                }

                if (input) {
                    copy.push({
                        sender: "user",
                        text: input,
                        runId: runID,
                        contentID,
                    });
                    return copy;
                }

                if (toolCall) {
                    return handleToolCallEvent(copy, event);
                }

                if (prompt) {
                    copy.push(promptMessage(prompt, runID));
                    return copy;
                }

                if (content) {
                    copy.push({
                        sender: "agent",
                        text: content,
                        runId: runID,
                        contentID,
                    });
                    return copy;
                }

                return copy;
            });
        },
        [onRunEvent]
    );

    useEffect(() => {
        setMessages([]);

        if (!threadId) return;

        let replayComplete = false;
        let replayMessages: ChatEvent[] = [];

        const source = ThreadsService.getThreadEventSource(threadId);
        source.addEventListener("close", source.close);

        source.onmessage = (chunk) => {
            const event = JSON.parse(chunk.data) as ChatEvent;

            if (event.replayComplete) {
                replayComplete = true;
                replayMessages.forEach(addContent);
                replayMessages = [];
            }

            if (!replayComplete) {
                replayMessages.push(event);
                return;
            }

            addContent(event);
        };

        return () => {
            source.close();
            setIsRunning(false);
        };
    }, [threadId, addContent]);

    return { messages, isRunning };
}

const findIndexLastPendingToolCall = (messages: Message[]) => {
    for (let i = messages.length - 1; i >= 0; i--) {
        const message = messages[i];
        if (message.tools && !message.tools[0].output) {
            return i;
        }
    }
    return null;
};

const handleToolCallEvent = (messages: Message[], event: ChatEvent) => {
    if (!event.toolCall) return messages;

    const { toolCall } = event;
    if (toolCall.output) {
        const index = findIndexLastPendingToolCall(messages);
        if (index !== null) {
            // update the found pending toolcall message (without output)
            messages[index].tools = [toolCall];
            return messages;
        }
    }

    // otherwise add a new toolcall message
    messages.push(toolCallMessage(toolCall));
    return messages;
};
