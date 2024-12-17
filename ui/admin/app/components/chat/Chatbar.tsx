import { ArrowUpIcon, SquareIcon } from "lucide-react";
import { useState } from "react";

import { cn } from "~/lib/utils";

import { ChatActions } from "~/components/chat/ChatActions";
import { useChat } from "~/components/chat/ChatContext";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Button } from "~/components/ui/button";
import { AutosizeTextarea } from "~/components/ui/textarea";

type ChatbarProps = {
    className?: string;
};

export function Chatbar({ className }: ChatbarProps) {
    const [input, setInput] = useState("");
    const { abortRunningThread, processUserMessage, isRunning, isInvoking } =
        useChat();

    const handleSubmit = (e: React.FormEvent) => {
        e.preventDefault();

        if (isRunning) {
            abortRunningThread();
        }

        if (input.trim()) {
            processUserMessage(input);
        }

        setInput("");
    };

    return (
        <form
            onSubmit={handleSubmit}
            className={cn("flex items-end gap-2", className)}
        >
            <div className="relative flex-grow">
                <AutosizeTextarea
                    className="rounded-3xl p-2"
                    variant="flat"
                    value={input}
                    onKeyDown={(e) => {
                        if (e.key === "Enter" && !e.shiftKey) {
                            e.preventDefault();
                            handleSubmit(e);
                        }
                    }}
                    maxHeight={200}
                    minHeight={0}
                    onChange={(e) => setInput(e.target.value)}
                    placeholder="Type your message..."
                    bottomContent={
                        <div className="flex flex-row-reverse items-center justify-between">
                            <Button
                                size="icon-sm"
                                className="m-2"
                                color="primary"
                                type="submit"
                                disabled={(!input && !isRunning) || isInvoking}
                            >
                                {isInvoking ? (
                                    <LoadingSpinner />
                                ) : isRunning ? (
                                    <SquareIcon className="fill-primary-foreground text-primary-foreground !w-3 !h-3" />
                                ) : (
                                    <ArrowUpIcon />
                                )}
                            </Button>

                            <ChatActions className="p-2" />
                        </div>
                    }
                />
            </div>
        </form>
    );
}
