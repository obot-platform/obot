import "@radix-ui/react-tooltip";
import { WrenchIcon } from "lucide-react";
import { useMemo } from "react";
import Markdown, { defaultUrlTransform } from "react-markdown";
import rehypeExternalLinks from "rehype-external-links";
import remarkGfm from "remark-gfm";

import { Message as MessageType } from "~/lib/model/messages";
import { cn } from "~/lib/utils";

import { MessageDebug } from "~/components/chat/MessageDebug";
import { CustomMarkdownComponents } from "~/components/react-markdown";
import { ToolIcon } from "~/components/tools/ToolIcon";
import { Button } from "~/components/ui/button";
import { Card } from "~/components/ui/card";
import { TypingDots } from "~/components/ui/typing-spinner";

import { ToolCallInfo } from "./ToolCallInfo";

interface MessageProps {
    message: MessageType;
}

// Allow links for file references in messages if it starts with file://, otherwise this will cause an empty href and cause app to reload when clicking on it
const urlTransformAllowFiles = (u: string) => {
    if (u.startsWith("file://")) {
        return u;
    }
    return defaultUrlTransform(u);
};

const OpenMarkdownLinkRegex = new RegExp(/\[([^\]]+)\]\(https?:\/\/[^)]*$/);

export function Message({ message }: MessageProps) {
    const isUser = message.sender === "user";

    // note(ryanhopperlowe) we only support one tool call per message for now
    // leaving it in case that changes in the future
    const [toolCall = null] = message.tools || [];

    const parsedMessage = useMemo(() => {
        if (OpenMarkdownLinkRegex.test(message.text)) {
            return message.text.replace(
                OpenMarkdownLinkRegex,
                (_, linkText) => `[${linkText}]()`
            );
        }
        return message.text;
    }, [message.text]);

    return (
        <div className="mb-4 w-full">
            <div
                className={cn("flex", isUser ? "justify-end" : "justify-start")}
            >
                {message.isLoading ? (
                    <TypingDots className="p-4" />
                ) : (
                    <Card
                        className={cn(
                            message.error &&
                                "border border-error bg-error-foreground",
                            "break-words overflow-hidden",
                            isUser
                                ? "max-w-[80%] bg-blue-500"
                                : "w-full max-w-full"
                        )}
                    >
                        <div className="max-w-full overflow-hidden p-4 flex gap-2 items-center pl-[20px]">
                            {toolCall?.metadata.icon && (
                                <ToolIcon
                                    icon={toolCall.metadata.icon}
                                    category={toolCall.metadata.category}
                                    name={toolCall.name}
                                    className="w-5 h-5"
                                />
                            )}

                            <Markdown
                                className={cn(
                                    "flex-auto max-w-full prose overflow-x-auto dark:prose-invert prose-pre:whitespace-pre-wrap prose-pre:break-words prose-thead:text-left prose-img:rounded-xl prose-img:shadow-lg break-words",
                                    { "text-white prose-invert": isUser }
                                )}
                                remarkPlugins={[remarkGfm]}
                                rehypePlugins={[
                                    [rehypeExternalLinks, { target: "_blank" }],
                                ]}
                                urlTransform={urlTransformAllowFiles}
                                components={CustomMarkdownComponents}
                            >
                                {parsedMessage ||
                                    "Waiting for more information..."}
                            </Markdown>

                            {toolCall && (
                                <ToolCallInfo tool={toolCall}>
                                    <Button variant="secondary" size="icon">
                                        <WrenchIcon className="w-4 h-4" />
                                    </Button>
                                </ToolCallInfo>
                            )}

                            {!isUser && message.runId && (
                                <div className="self-start">
                                    <MessageDebug
                                        variant="secondary"
                                        runId={message.runId}
                                    />
                                </div>
                            )}

                            {/* this is a hack to take up space for the debug button */}
                            {!toolCall && !message.runId && !isUser && (
                                <div className="invisible">
                                    <Button size="icon" />
                                </div>
                            )}
                        </div>
                    </Card>
                )}
            </div>
        </div>
    );
}
