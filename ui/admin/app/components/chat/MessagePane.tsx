import { ToolCall } from "~/lib/model/chatEvents";
import { Message as MessageType } from "~/lib/model/messages";
import { cn } from "~/lib/utils";

import { useChat } from "~/components/chat/ChatContext";
import { Message } from "~/components/chat/Message";
import { NoMessages } from "~/components/chat/NoMessages";
import { ScrollArea } from "~/components/ui/scroll-area";
import { TypingDots } from "~/components/ui/typing-spinner";

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
        <div className={cn("flex flex-col h-full", className, classNames.root)}>
            <ScrollArea
                startScrollAt="bottom"
                enableScrollTo="bottom"
                enableScrollStick="bottom"
                className={cn("h-full w-full relative", classNames.messageList)}
            >
                {isEmpty ? (
                    <NoMessages />
                ) : (
                    <div className="p-4 space-y-6 w-full">
                        {messages.map((message, i) => (
                            <Message key={i} message={message} />
                        ))}

                        <TypingDots
                            className={cn("p-4", {
                                invisible: !isRunning,
                            })}
                        />
                    </div>
                )}
            </ScrollArea>
        </div>
    );
}
