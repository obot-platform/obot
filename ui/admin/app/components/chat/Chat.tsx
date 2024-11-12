import { useState } from "react";

import { cn } from "~/lib/utils";

import { useChat } from "~/components/chat/ChatContext";
import { Chatbar } from "~/components/chat/Chatbar";
import { MessagePane } from "~/components/chat/MessagePane";
import { Button } from "~/components/ui/button";

type ChatProps = React.HTMLAttributes<HTMLDivElement> & {
    showStartButton?: boolean;
};

export function Chat({ className }: ChatProps) {
    const {
        messages,
        threadId,
        mode,
        invoke,
        readOnly,
        isInvoking,
        isRunning,
    } = useChat();
    const [runTriggered, setRunTriggered] = useState(false);

    const showMessagePane =
        mode === "agent" ||
        (mode === "workflow" && (threadId || runTriggered || !readOnly));

    const showStartButtonPane = mode === "workflow" && !readOnly;

    return (
        <div className={`flex flex-col h-full ${className}`}>
            {showMessagePane && (
                <div className="flex-grow overflow-hidden">
                    <MessagePane
                        classNames={{ root: "h-full", messageList: "px-20" }}
                        messages={messages}
                    />
                </div>
            )}

            {mode === "agent" && !readOnly && <Chatbar className="px-20" />}

            {showStartButtonPane && (
                <div
                    className={cn("px-20 mb-4", {
                        "flex justify-center items-center h-full": !threadId,
                    })}
                >
                    <Button
                        variant="secondary"
                        onClick={() => {
                            setRunTriggered(true);
                            invoke();
                        }}
                        className={cn({
                            "w-full": threadId,
                        })}
                        loading={isInvoking || isRunning}
                        disabled={isInvoking || isRunning}
                    >
                        Run
                    </Button>
                </div>
            )}
        </div>
    );
}
