import { LibraryIcon, WrenchIcon } from "lucide-react";
import { useMemo } from "react";
import useSWR from "swr";

import { Agent } from "~/lib/model/agents";
import { KnowledgeFile } from "~/lib/model/knowledge";
import { AgentService } from "~/lib/service/api/agentService";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { cn } from "~/lib/utils";

import { TypographyMuted, TypographySmall } from "~/components/Typography";
import { ToolEntry } from "~/components/agent/ToolEntry";
import { useChat } from "~/components/chat/ChatContext";
import { Button } from "~/components/ui/button";
import {
    Popover,
    PopoverContent,
    PopoverTrigger,
} from "~/components/ui/popover";
import { Switch } from "~/components/ui/switch";

export function ChatHelpers() {
    const { threadId } = useChat();

    const { data: thread } = useSWR(
        ThreadsService.getThreadById.key(threadId),
        ({ threadId }) => ThreadsService.getThreadById(threadId)
    );

    const { data: knowledge } = useSWR(
        ThreadsService.getKnowledge.key(threadId),
        ({ threadId }) => ThreadsService.getKnowledge(threadId)
    );

    const { data: agent } = useSWR(
        AgentService.getAgentById.key(thread?.agentID),
        ({ agentId }) => AgentService.getAgentById(agentId)
    );

    const tools = thread?.tools;

    console.log(knowledge);

    return (
        <div className="w-full flex items-center px-20 py-2">
            <div className="flex items-center gap-2">
                <ToolsInfo
                    tools={tools ?? []}
                    agent={agent}
                    disabled={!thread}
                />

                <KnowledgeInfo knowledge={knowledge ?? []} disabled={!thread} />
            </div>
        </div>
    );
}

function ToolsInfo({
    tools,
    className,
    agent,
    disabled,
}: {
    tools: string[];
    className?: string;
    agent: Nullish<Agent>;
    disabled?: boolean;
}) {
    const toolItems = useMemo(() => {
        if (!agent)
            return tools.map((tool) => ({
                tool,
                isToggleable: false,
                isEnabled: true,
            }));

        const agentTools = (agent.tools ?? []).map((tool) => ({
            tool,
            isToggleable: false,
            isEnabled: true,
        }));

        const { defaultThreadTools, availableThreadTools } = agent ?? {};

        const toggleableTools = [
            ...(defaultThreadTools ?? []),
            ...(availableThreadTools ?? []),
        ].map((tool) => ({
            tool,
            isToggleable: true,
            isEnabled: tools.includes(tool),
        }));

        return [...agentTools, ...toggleableTools];
    }, [tools, agent]);

    return (
        <Popover>
            <PopoverTrigger asChild>
                <Button
                    size="sm"
                    variant="secondary"
                    className={cn("gap-2", className)}
                    startContent={<WrenchIcon />}
                    disabled={disabled}
                >
                    Tools
                </Button>
            </PopoverTrigger>

            <PopoverContent className="w-80">
                {toolItems.length > 0 ? (
                    <div className="space-y-2">
                        <TypographySmall className="font-semibold">
                            Available Tools
                        </TypographySmall>
                        <div className="space-y-1">
                            {toolItems.map(
                                ({ tool, isToggleable, isEnabled }) => (
                                    <ToolEntry
                                        key={tool}
                                        tool={tool}
                                        actions={
                                            isToggleable ? (
                                                <Switch
                                                    checked={isEnabled}
                                                    disabled
                                                    onCheckedChange={() => {}}
                                                />
                                            ) : (
                                                <TypographyMuted>
                                                    On
                                                </TypographyMuted>
                                            )
                                        }
                                    />
                                )
                            )}
                        </div>
                    </div>
                ) : (
                    <TypographyMuted>No tools available</TypographyMuted>
                )}
            </PopoverContent>
        </Popover>
    );
}

function KnowledgeInfo({
    knowledge,
    className,
    disabled,
}: {
    knowledge: KnowledgeFile[];
    className?: string;
    disabled?: boolean;
}) {
    return (
        <Popover>
            <PopoverTrigger asChild>
                <Button
                    size="sm"
                    variant="secondary"
                    className={cn("gap-2", className)}
                    startContent={<LibraryIcon />}
                    disabled={disabled}
                >
                    Knowledge
                </Button>
            </PopoverTrigger>

            <PopoverContent>
                {knowledge.length > 0 ? (
                    <div className="space-y-2">
                        {knowledge.map((file) => (
                            <TypographyMuted key={file.id}>
                                {file.fileName}
                            </TypographyMuted>
                        ))}
                    </div>
                ) : (
                    <TypographyMuted>No knowledge available</TypographyMuted>
                )}
            </PopoverContent>
        </Popover>
    );
}
