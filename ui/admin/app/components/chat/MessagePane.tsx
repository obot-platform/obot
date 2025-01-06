import { ToolCall } from "~/lib/model/chatEvents";
import { Message as MessageType } from "~/lib/model/messages";
import { cn } from "~/lib/utils";

import { useChat } from "~/components/chat/ChatContext";
import { Message } from "~/components/chat/Message";
import { MessageDebug } from "~/components/chat/MessageDebug";
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

    const currentRunId = messages.findLast((message) => message.runId)?.runId;

    return (
        <div className={cn("flex flex-col h-full", className, classNames.root)}>
            <ScrollArea
                startScrollAt="bottom"
                enableScrollTo="bottom"
                enableScrollStick="bottom"
                classNames={{
                    root: cn("h-full w-full relative", classNames.messageList),
                    viewport: cn(isEmpty && "flex flex-col justify-center"),
                }}
            >
                {isEmpty ? (
                    <NoMessages />
                ) : (
                    <div className="p-4 space-y-6 w-full">
                        {messages.map((message, i) => (
                            <Message key={i} message={message} />
                        ))}

                        <div
                            className={cn(
                                "p-4 flex items-center justify-between gap-4 w-full",
                                { invisible: !isRunning }
                            )}
                        >
                            <TypingDots />

                            {currentRunId && (
                                <MessageDebug runId={currentRunId} />
                            )}
                        </div>
                    </div>
                )}
            </ScrollArea>
        </div>
    );
}
