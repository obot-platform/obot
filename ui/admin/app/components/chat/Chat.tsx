import { ComponentProps } from "react";

import { cn } from "~/lib/utils";

import { useChat } from "~/components/chat/ChatContext";
import { Chatbar } from "~/components/chat/Chatbar";
import { MessagePane } from "~/components/chat/MessagePane";

type ChatProps = {
	className?: string;
	classNames?: {
		root?: string;
		messagePane?: ComponentProps<typeof MessagePane>["classNames"];
	};
};

export function Chat({ className, classNames }: ChatProps) {
	const { messages, mode, readOnly } = useChat();

	const showMessagePane = mode === "agent";

	return (
		<div className={cn("flex h-full flex-col pb-5", className)}>
			{showMessagePane && (
				<div className="flex-grow overflow-hidden">
					<MessagePane
						classNames={{
							...classNames?.messagePane,
							root: cn("h-full", classNames?.messagePane?.root),
							messageList: cn("px-20", classNames?.messagePane?.messageList),
						}}
						messages={messages}
					/>
				</div>
			)}

			{mode === "agent" && !readOnly && <Chatbar className="px-20" />}
		</div>
	);
}
