import { ReactNode, createContext, useContext, useMemo } from "react";
import { mutate } from "swr";

import { Message } from "~/lib/model/messages";
import { InvokeService } from "~/lib/service/api/invokeService";
import { ThreadsService } from "~/lib/service/api/threadsService";

import { useMessageStream } from "~/hooks/messages/useMessageSource";
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

	const source = useMemo(
		() => (threadId ? ThreadsService.getThreadEventSource(threadId) : null),
		[threadId]
	);

	const { messages, isRunning } = useMessageStream(source);

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
