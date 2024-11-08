import "@radix-ui/react-tooltip";
import { WrenchIcon } from "lucide-react";
import React, { useMemo } from "react";
import Markdown, { defaultUrlTransform } from "react-markdown";
import rehypeExternalLinks from "rehype-external-links";
import remarkGfm from "remark-gfm";

import { OAuthPrompt } from "~/lib/model/chatEvents";
import { Message as MessageType } from "~/lib/model/messages";
import { cn } from "~/lib/utils";

import { TypographyP } from "~/components/Typography";
import { MessageDebug } from "~/components/chat/MessageDebug";
import { ToolCallInfo } from "~/components/chat/ToolCallInfo";
import { CustomMarkdownComponents } from "~/components/react-markdown";
import { ToolIcon } from "~/components/tools/ToolIcon";
import { Button } from "~/components/ui/button";

interface MessageProps {
    message: MessageType;
    isRunning?: boolean;
}

// Allow links for file references in messages if it starts with file://, otherwise this will cause an empty href and cause app to reload when clicking on it
const urlTransformAllowFiles = (u: string) => {
    if (u.startsWith("file://")) {
        return u;
    }
    return defaultUrlTransform(u);
};

const OpenMarkdownLinkRegex = new RegExp(/\[([^\]]+)\]\(https?:\/\/[^)]*$/);

export const Message = React.memo(({ message }: MessageProps) => {
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
                <div
                    className={cn(
                        "break-words overflow-hidden flex flex-col justify-start rounded-2xl",
                        message.error && "bg-error-foreground",
                        isUser
                            ? "max-w-[80%] bg-secondary"
                            : "w-full max-w-full"
                    )}
                >
                    <div className="max-w-full overflow-hidden p-4 flex gap-2 items-center pl-[20px]">
                        {toolCall?.metadata?.icon && (
                            <ToolIcon
                                icon={toolCall.metadata.icon}
                                category={toolCall.metadata.category}
                                name={toolCall.name}
                                className="w-5 h-5"
                            />
                        )}

                        {message.prompt?.metadata ? (
                            <PromptMessage prompt={message.prompt} />
                        ) : (
                            <Markdown
                                className={cn(
                                    "flex-auto max-w-full prose overflow-x-auto dark:prose-invert prose-pre:whitespace-pre-wrap prose-pre:break-words prose-thead:text-left prose-img:rounded-xl prose-img:shadow-lg break-words",
                                    {
                                        "text-secondary-foreground prose-invert":
                                            isUser,
                                    }
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
                        )}

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
                </div>
            </div>
        </div>
    );
});

Message.displayName = "Message";

function PromptMessage({ prompt }: { prompt: OAuthPrompt }) {
    if (!prompt.metadata) return null;

    return (
        <div className="flex-auto flex flex-col flex-wrap items-start gap-2 w-fit">
            <TypographyP className="min-w-fit">
                <b>
                    {[prompt.metadata?.category, prompt.name]
                        .filter(Boolean)
                        .join(" - ")}
                </b>
                {": "}
                Tool Call requires authentication
            </TypographyP>

            <Button asChild variant="secondary">
                <a
                    rel="noreferrer"
                    target="_blank"
                    href={prompt.metadata?.authURL}
                    className="flex items-center gap-2 w-fit"
                >
                    <ToolIcon
                        icon={prompt.metadata?.icon}
                        category={prompt.metadata?.category}
                        name={prompt.name}
                        className="w-5 h-5"
                        disableTooltip
                    />
                    Authenticate with {prompt.metadata?.category}
                </a>
            </Button>
        </div>
    );
}
