import { useCallback, useState } from "react";

import { ChatEvent } from "~/lib/model/chatEvents";

import { useToolReference } from "~/components/agent/ToolEntry";
import { Chat, ChatProvider } from "~/components/chat";
import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogClose,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from "~/components/ui/dialog";

type AgentAuthenticationDialogProps = {
    threadId: Nullish<string>;
    onComplete: () => void;
    entityId: string;
    tool: string;
};

export function AgentAuthenticationDialog({
    onComplete,
    threadId,
    entityId,
    tool,
}: AgentAuthenticationDialogProps) {
    const [done, setDone] = useState(false);
    const handleDone = useCallback(() => setDone(true), []);

    const { icon, label } = useToolReference(tool);

    const onRunEvent = useCallback(
        ({ content }: ChatEvent) => {
            if (content === "DONE") handleDone();
        },
        [handleDone]
    );

    return (
        <Dialog open={!!threadId} onOpenChange={onComplete}>
            <DialogContent>
                <DialogHeader>
                    <DialogTitle className="flex items-center gap-2">
                        {icon} <span>Authorize {label}</span>
                    </DialogTitle>

                    <DialogDescription hidden={done}></DialogDescription>
                </DialogHeader>

                {done ? (
                    <DialogDescription>
                        {label} has successfully been authorized. You may now
                        close this modal.
                    </DialogDescription>
                ) : (
                    <ChatProvider
                        threadId={threadId}
                        id={entityId}
                        readOnly
                        onRunEvent={onRunEvent}
                    >
                        <Chat
                            classNames={{
                                messagePane: { messageList: "px-0" },
                            }}
                        />
                    </ChatProvider>
                )}

                <DialogFooter>
                    <DialogClose asChild>
                        <Button variant={done ? "default" : "secondary"}>
                            {done ? "Done" : "Cancel"}
                        </Button>
                    </DialogClose>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
