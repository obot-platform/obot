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
}: {
    children: ReactNode;
    mode?: Mode;
    id: string;
    threadId?: Nullish<string>;
    onCreateThreadId?: (threadId: string) => void;
    readOnly?: boolean;
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

    const { messages, isRunning } = useMessageSource(threadId);

    return (
        <ChatContext.Provider
            value={{
                messages,
                processUserMessage: invoke,
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

function useMessageSource(threadId?: Nullish<string>) {
    const [messages, setMessages] = useState<Message[]>([]);
    const [isRunning, setIsRunning] = useState(false);

    const addContent = useCallback((event: ChatEvent) => {
        const {
            content,
            prompt,
            toolCall,
            runComplete,
            input,
            error,
            runID,
            contentID,
        } = event;

        setIsRunning(!runComplete);

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
                copy.push(toolCallMessage(toolCall));
                return copy;
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
    }, []);

    useEffect(() => {
        setMessages([]);

        if (!threadId) return;

        const source = ThreadsService.getThreadEventSource(threadId);

        let replayComplete = false;
        let replayMessages: ChatEvent[] = [];

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
