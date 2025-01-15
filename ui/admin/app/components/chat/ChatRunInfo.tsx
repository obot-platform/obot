import { Message } from "~/lib/model/messages";
import { cn } from "~/lib/utils";

import { MessageDebug } from "~/components/chat/MessageDebug";
import { TypingDots } from "~/components/ui/typing-spinner";

type ChatRunInfoProps = {
	messages: Message[];
	isRunning: boolean;
};

export function ChatRunInfo({ messages, isRunning }: ChatRunInfoProps) {
	const currentRunId = messages.findLast((message) => !!message.runId)?.runId;

	if (!isRunning) return null;

	return (
		<div className={cn("flex gap-4")}>
			<TypingDots />

			{currentRunId && <MessageDebug runId={currentRunId} />}
		</div>
	);
}
