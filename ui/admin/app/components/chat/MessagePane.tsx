import { useEffect, useRef, useState } from "react";

import { ToolCall } from "~/lib/model/chatEvents";
import { Message as MessageType } from "~/lib/model/messages";
import { cn } from "~/lib/utils";

import { useChat } from "~/components/chat/ChatContext";
import { Message } from "~/components/chat/Message";
import { NoMessages } from "~/components/chat/NoMessages";
import { useTheme } from "~/components/theme";
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
	const [shouldCenter, setShouldCenter] = useState(true);
	const noMessagesRef = useRef<HTMLDivElement>(null);
	const { readOnly, isRunning, mode, icons, name } = useChat();
	const { theme } = useTheme();
	const isDarkMode = theme === "dark";

	const isEmpty = messages.length === 0 && !readOnly && mode === "agent";

	useEffect(() => {
		if (isEmpty && noMessagesRef.current) {
			const parentHeight =
				noMessagesRef.current.parentElement?.parentElement?.parentElement
					?.clientHeight || 0;
			const elementHeight = noMessagesRef.current.clientHeight;
			setShouldCenter(elementHeight < parentHeight);
		}
	}, [isEmpty]);

	return (
		<div className={cn("flex h-full flex-col", className, classNames.root)}>
			<ScrollArea
				startScrollAt="bottom"
				enableScrollTo="bottom"
				enableScrollStick="bottom"
				classNames={{
					root: cn("relative h-full w-full", classNames.messageList),
					viewport: cn(
						isEmpty && shouldCenter && "flex flex-col justify-center"
					),
				}}
			>
				{isEmpty ? (
					<div ref={noMessagesRef}>
						<NoMessages />
					</div>
				) : (
					<div className="w-full space-y-6 py-6">
						{messages.map((message, i) => (
							<Message
								key={i}
								message={message}
								isRunning={isRunning}
								icons={icons}
								isDarkMode={isDarkMode}
								isMostRecent={i === messages.length - 1}
								name={name}
							/>
						))}
					</div>
				)}
			</ScrollArea>
		</div>
	);
}
