import { useState } from "react";

import { useChat } from "~/components/chat/ChatContext";
import { Chatbar } from "~/components/chat/Chatbar";
import { MessagePane } from "~/components/chat/MessagePane";
import { Button } from "~/components/ui/button";

type ChatProps = React.HTMLAttributes<HTMLDivElement> & {
    showStartButton?: boolean;
};

export function Chat({ className, showStartButton = false }: ChatProps) {
    const { messages, threadId, generatingMessage, mode, invoke, readOnly } =
        useChat();
    const [runTriggered, setRunTriggered] = useState(false);

    const showMessagePane =
        mode === "agent" ||
        (mode === "workflow" && (threadId || runTriggered || !showStartButton));

    const showStartButtonPane =
        mode === "workflow" && showStartButton && !(threadId || runTriggered);

    return (
        <div className={`flex flex-col h-full ${className}`}>
            {showMessagePane && (
                <div className="flex-grow overflow-hidden">
                    <MessagePane
                        classNames={{ root: "h-full", messageList: "px-20" }}
                        messages={messages}
                        generatingMessage={generatingMessage}
                    />
                </div>
            )}

            {mode === "agent" && !readOnly && <Chatbar className="px-20" />}

            {showStartButtonPane && (
                <div className="flex justify-center items-center h-full px-20">
                    <Button
                        variant="secondary"
                        onClick={() => {
                            setRunTriggered(true);
                            invoke();
                        }}
                    >
                        Run
                    </Button>
                </div>
            )}
        </div>
    );
}
