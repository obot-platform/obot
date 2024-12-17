import { cn } from "~/lib/utils";

import { useChat } from "~/components/chat/ChatContext";
import { FilesInfo } from "~/components/chat/chat-actions/FilesInfo";
import { KnowledgeInfo } from "~/components/chat/chat-actions/KnowledgeInfo";
import { TablesInfo } from "~/components/chat/chat-actions/TablesInfo";
import { ToolsInfo } from "~/components/chat/chat-actions/ToolsInfo";
import {
    useOptimisticThread,
    useThreadAgents as useThreadAgent,
    useThreadKnowledge,
} from "~/components/chat/thread-helpers";

export function ChatActions({ className }: { className?: string }) {
    const { threadId } = useChat();

    const { data: knowledge } = useThreadKnowledge(threadId);
    const { data: agent } = useThreadAgent(threadId);
    const { thread, updateThread } = useOptimisticThread(threadId);

    const tools = thread?.tools;

    return (
        <div className={cn("w-full flex items-center", className)}>
            <div className="flex items-center gap-2">
                <ToolsInfo
                    tools={tools ?? []}
                    onChange={(tools) => updateThread({ tools })}
                    agent={agent}
                    disabled={!thread}
                />

                <KnowledgeInfo knowledge={knowledge ?? []} disabled={!thread} />

                <FilesInfo />

                <TablesInfo />
            </div>
        </div>
    );
}
