import { ToolCall } from "~/lib/model/chatEvents";
import { Message as MessageType } from "~/lib/model/messages";
import { cn } from "~/lib/utils";

import { Message } from "~/components/chat/Message";
import { NoMessages } from "~/components/chat/NoMessages";
import { ScrollArea } from "~/components/ui/scroll-area";

import { useChat } from "./ChatContext";

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
    generatingMessage,
}: MessagePaneProps) {
    const { mode, readOnly } = useChat();

    return (
        <div className={cn("flex flex-col h-full", className, classNames.root)}>
            {messages.length === 0 &&
            !generatingMessage &&
            mode === "agent" &&
            !readOnly ? (
                <div className="flex-grow flex items-center justify-center">
                    <NoMessages />
                </div>
            ) : (
                <ScrollArea
                    startScrollAt="bottom"
                    enableScrollTo="bottom"
                    enableScrollStick="bottom"
                    className={cn(
                        "h-full w-full relative",
                        classNames.messageList
                    )}
                >
                    <div className="p-4 space-y-6 w-full">
                        {messages.map((message, i) => (
                            <Message key={i} message={message} />
                        ))}
                        {generatingMessage && (
                            <Message message={generatingMessage} />
                        )}
                    </div>
                </ScrollArea>
            )}
        </div>
    );
}
