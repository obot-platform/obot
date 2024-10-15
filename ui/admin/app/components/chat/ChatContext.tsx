import {
    ReactNode,
    createContext,
    startTransition,
    useContext,
    useEffect,
    useMemo,
    useRef,
    useState,
} from "react";
import useSWR, { mutate } from "swr";

import { ChatEvent, combineChatEvents } from "~/lib/model/chatEvents";
import {
    Message,
    chatEventsToMessages,
    toolCallMessage,
} from "~/lib/model/messages";
import { InvokeService } from "~/lib/service/api/invokeService";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { readStream } from "~/lib/stream";

import { useAsync } from "~/hooks/useAsync";

type Mode = "agent" | "workflow";

interface ChatContextType {
    messages: Message[];
    mode: Mode;
    processUserMessage: (text: string, sender: "user" | "agent") => void;
    id: string;
    threadId: string | undefined;
    generatingMessage: Message | null;
    invoke: (prompt?: string) => void;
    readOnly?: boolean;
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
    threadId?: string;
    onCreateThreadId?: (threadId: string) => void;
    readOnly?: boolean;
}) {
    const [insertedMessages, setInsertedMessages] = useState<Message[]>([]);
    const [generatingMessage, setGeneratingMessage] = useState<string | null>(
        null
    );
    const isRunningToolCall = useRef(false);
    // todo(tylerslaton): this is a huge hack to get the generating message and runId to be
    // interactable during workflow invokes. take a look at invokeWorkflow to see why this is
    // currently needed.
    const generatingRunIdRef = useRef<string | null>(null);
    const generatingMessageRef = useRef<string | null>(null);

    const appendToGeneratingMessage = (content: string) => {
        generatingMessageRef.current =
            (generatingMessageRef.current || "") + content;

        setGeneratingMessage(generatingMessageRef.current);
    };

    const clearGeneratingMessage = () => {
        generatingMessageRef.current = null;
        setGeneratingMessage(null);
    };

    const getThreadEvents = useSWR(
        ThreadsService.getThreadEvents.key(threadId),
        ({ threadId }) => ThreadsService.getThreadEvents(threadId),
        {
            onSuccess: () => setInsertedMessages([]),
            revalidateIfStale: false,
            revalidateOnFocus: false,
            revalidateOnReconnect: false,
        }
    );

    const messages = useMemo(
        () => chatEventsToMessages(getThreadEvents.data || []),
        [getThreadEvents.data]
    );

    // clear out inserted messages when the threadId changes
    useEffect(() => setInsertedMessages([]), [threadId]);

    /** inserts message optimistically */
    const insertMessage = (message: Message) => {
        setInsertedMessages((prev) => [...prev, message]);
    };

    /**
     * processUserMessage is responsible for adding the user's message to the chat and
     * triggering the agent to respond to it.
     */
    const processUserMessage = (text: string, sender: "user" | "agent") => {
        if (mode === "workflow" || readOnly) return;
        const newMessage: Message = { text, sender };

        insertMessage(newMessage);
        handlePrompt(newMessage.text);
    };

    const invoke = (prompt?: string) => {
        if (prompt && mode === "agent" && !readOnly) {
            handlePrompt(prompt);
        }
    };

    const handlePrompt = (prompt: string) => {
        if (prompt && mode === "agent" && !readOnly) {
            invokeAgent.execute({
                slug: id,
                prompt: prompt,
                thread: threadId,
            });
        }
        // do nothing if the mode is workflow
    };

    const insertGeneratingMessage = (runId?: string) => {
        // skip if there is no message or it is only whitespace
        if (generatingMessageRef.current) {
            insertMessage({
                sender: "agent",
                runId,
                text: generatingMessageRef.current,
            });
            clearGeneratingMessage();
        }
    };

    const invokeAgent = useAsync(InvokeService.invokeAgentWithStream, {
        onSuccess: ({ reader, threadId: responseThreadId }) => {
            clearGeneratingMessage();

            readStream<ChatEvent>({
                reader,
                onChunk: (chunk) =>
                    // use a transition for performance
                    startTransition(() => {
                        const { content, toolCall, runID, input } = chunk;

                        generatingRunIdRef.current = runID;

                        if (toolCall) {
                            isRunningToolCall.current = true;
                            // cut off generating message
                            insertGeneratingMessage(runID);

                            // insert tool call message
                            insertMessage(toolCallMessage(toolCall));

                            clearGeneratingMessage();

                            return;
                        }

                        isRunningToolCall.current = false;

                        if (content && !input) {
                            appendToGeneratingMessage(content);
                        }
                    }),
                onComplete: async (chunks) => {
                    const compactEvents = combineChatEvents(chunks);

                    if (responseThreadId && !threadId) {
                        // if this is a new thread, persist it by
                        // prepopulating the cache with the events before setting the threadId
                        // to avoid a flash of no messages
                        await mutate(
                            ThreadsService.getThreadEvents.key(
                                responseThreadId
                            ),
                            compactEvents,
                            { revalidate: false }
                        );

                        // persist the threadId
                        onCreateThreadId?.(responseThreadId);

                        // revalidate threads
                        mutate(ThreadsService.getThreads.key());
                        clearGeneratingMessage();
                    } else {
                        insertGeneratingMessage(chunks[0]?.runID);
                    }

                    invokeAgent.clear();
                    generatingRunIdRef.current = null;
                },
            });
        },
    });

    const outGeneratingMessage = useMemo<Message | null>(() => {
        if (invokeAgent.isLoading)
            return { sender: "agent", text: "", isLoading: true };

        // slice the first character because it is always a newline for some reason
        if (!generatingMessage) {
            if (invokeAgent.data?.reader && !isRunningToolCall.current) {
                return {
                    sender: "agent",
                    text: "",
                    isLoading: true,
                };
            }

            return null;
        }

        return {
            sender: "agent",
            text: generatingMessage,
            runId: generatingRunIdRef.current ?? undefined,
        };
    }, [generatingMessage, invokeAgent.isLoading, invokeAgent.data]);

    // combine messages and inserted messages
    const outMessages = useMemo(() => {
        return [...(messages ?? []), ...insertedMessages];
    }, [messages, insertedMessages]);

    return (
        <ChatContext.Provider
            value={{
                messages: outMessages,
                processUserMessage,
                mode,
                id,
                threadId,
                generatingMessage: outGeneratingMessage,
                invoke,
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
