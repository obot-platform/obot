import { ComponentProps, useState } from "react";

import { cn } from "~/lib/utils";

import { useChat } from "~/components/chat/ChatContext";
import { Chatbar } from "~/components/chat/Chatbar";
import { MessagePane } from "~/components/chat/MessagePane";
import { RunWorkflow } from "~/components/chat/RunWorkflow";

type ChatProps = {
    className?: string;
    classNames?: {
        root?: string;
        messagePane?: ComponentProps<typeof MessagePane>["classNames"];
    };
};

export function Chat({ className, classNames }: ChatProps) {
    const { id, messages, threadId, mode, invoke, readOnly } = useChat();
    const [runTriggered, setRunTriggered] = useState(false);

    const showMessagePane =
        mode === "agent" ||
        (mode === "workflow" && (threadId || runTriggered || !readOnly));

    const showStartButtonPane = mode === "workflow" && !readOnly;

    return (
        <div className={`flex flex-col h-full pb-5 ${className}`}>
            {showMessagePane && (
                <div className="flex-grow overflow-hidden">
                    <MessagePane
                        classNames={{
                            ...classNames?.messagePane,
                            root: cn("h-full", classNames?.messagePane?.root),
                            messageList: cn(
                                "px-20",
                                classNames?.messagePane?.messageList
                            ),
                        }}
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
                    <RunWorkflow
                        workflowId={id}
                        onSubmit={(params) => {
                            setRunTriggered(true);
                            invoke(params && JSON.stringify(params));
                        }}
                        className={cn({ "w-full": threadId })}
                        popoverContentProps={{
                            className: cn({ "translate-y-[-50%]": !threadId }),
                        }}
                    >
                        Run
                    </RunWorkflow>
                </div>
            )}
        </div>
    );
}
