import { ArrowUpIcon } from "lucide-react";
import { useState } from "react";

import { cn } from "~/lib/utils";

import { useChat } from "~/components/chat/ChatContext";
import { ChatHelpers } from "~/components/chat/ChatHelpers";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Button } from "~/components/ui/button";
import { AutosizeTextarea } from "~/components/ui/textarea";

type ChatbarProps = {
    className?: string;
};

export function Chatbar({ className }: ChatbarProps) {
    const [input, setInput] = useState("");
    const { processUserMessage, isRunning, isInvoking } = useChat();

    const handleSubmit = (e: React.FormEvent) => {
        e.preventDefault();

        if (isRunning) return;

        if (input.trim()) {
            processUserMessage(input);
            setInput("");
        }
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
                                disabled={!input || isRunning || isInvoking}
                            >
                                {isInvoking ? (
                                    <LoadingSpinner />
                                ) : (
                                    <ArrowUpIcon />
                                )}
                            </Button>

                            <ChatHelpers className="p-2" />
                        </div>
                    }
                />
            </div>
        </form>
    );
}
