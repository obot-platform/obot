import { ToolCall } from "~/lib/model/chatEvents";
import { Message as MessageType } from "~/lib/model/messages";
import { cn } from "~/lib/utils";

import { useChat } from "~/components/chat/ChatContext";
import { Message } from "~/components/chat/Message";
import { NoMessages } from "~/components/chat/NoMessages";
import { ScrollArea } from "~/components/ui/scroll-area";

interface MessagePaneProps {
	messages: MessageType[];
	className?: string;
	classNames?: {
		root?: string;
		messageList?: string;
	};
	generatingMessage?: Nullish<MessageType>;
	generatingTools?: ToolCall[];
}

export function MessagePane({
	messages,
	className,
	classNames = {},
}: MessagePaneProps) {
	const { readOnly, isRunning, mode } = useChat();

	const isEmpty = messages.length === 0 && !readOnly && mode === "agent";

	return (
		<div className={cn("flex h-full flex-col", className, classNames.root)}>
			<ScrollArea
				startScrollAt="bottom"
				enableScrollTo="bottom"
				enableScrollStick="bottom"
				classNames={{
					root: cn("relative h-full w-full", classNames.messageList),
					viewport: cn(isEmpty && "flex flex-col justify-center"),
				}}
			>
				{isEmpty ? (
					<NoMessages />
				) : (
					<div className="w-full space-y-6 p-4">
						{messages.map((message, i) => (
							<Message key={i} message={message} isRunning={isRunning} />
						))}
					</div>
				)}
			</ScrollArea>
		</div>
	);
}
