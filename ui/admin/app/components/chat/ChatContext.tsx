import { ReactNode, createContext, useContext } from "react";
import { mutate } from "swr";

import { AgentIcons } from "~/lib/model/agents";
import { Message } from "~/lib/model/messages";
import { InvokeService } from "~/lib/service/api/invokeService";
import { ThreadsService } from "~/lib/service/api/threadsService";

import { useThreadEvents } from "~/hooks/messages/useThreadEvents";
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
	introductionMessage?: string;
	starterMessages?: string[];
	agentName?: string;
	icons?: AgentIcons | null;
}

const ChatContext = createContext<ChatContextType | undefined>(undefined);

export function ChatProvider({
	children,
	id,
	mode = "agent",
	threadId,
	onCreateThreadId,
	readOnly,
	introductionMessage,
	starterMessages,
	agentName,
	icons,
}: {
	children: ReactNode;
	mode?: Mode;
	id: string;
	threadId?: Nullish<string>;
	onCreateThreadId?: (threadId: string) => void;
	readOnly?: boolean;
	introductionMessage?: string;
	starterMessages?: string[];
	agentName?: string;
	icons?: AgentIcons | null;
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

	const { messages, isRunning } = useThreadEvents(threadId);

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
				introductionMessage,
				starterMessages,
				agentName,
				icons,
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
