import {
    LibraryIcon,
    PaperclipIcon,
    TableIcon,
    WrenchIcon,
} from "lucide-react";
import { useMemo } from "react";

import { Agent } from "~/lib/model/agents";
import { KnowledgeFile } from "~/lib/model/knowledge";
import { cn } from "~/lib/utils";

import { TypographyMuted, TypographySmall } from "~/components/Typography";
import { ToolEntry } from "~/components/agent/ToolEntry";
import { useChat } from "~/components/chat/ChatContext";
import {
    useOptimisticThread,
    useThreadAgents as useThreadAgent,
    useThreadKnowledge,
} from "~/components/chat/thread-helpers";
import { Button } from "~/components/ui/button";
import {
    Popover,
    PopoverContent,
    PopoverTrigger,
} from "~/components/ui/popover";
import { Switch } from "~/components/ui/switch";
import {
    Tooltip,
    TooltipContent,
    TooltipTrigger,
} from "~/components/ui/tooltip";

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

type ToolItem = {
    tool: string;
    isToggleable: boolean;
    isEnabled: boolean;
};

function ToolsInfo({
    tools,
    className,
    agent,
    disabled,
    onChange,
}: {
    tools: string[];
    className?: string;
    agent: Nullish<Agent>;
    disabled?: boolean;
    onChange: (tools: string[]) => void;
}) {
    const toolItems = useMemo<ToolItem[]>(() => {
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

    const handleToggleTool = (tool: string, checked: boolean) => {
        onChange(checked ? [...tools, tool] : tools.filter((t) => t !== tool));
    };

    return (
        <Tooltip>
            <TooltipContent>Tools</TooltipContent>

            <Popover>
                <TooltipTrigger asChild>
                    <PopoverTrigger asChild>
                        <Button
                            size="icon-sm"
                            variant="outline"
                            className={cn("gap-2", className)}
                            startContent={<WrenchIcon />}
                            disabled={disabled}
                        />
                    </PopoverTrigger>
                </TooltipTrigger>

                <PopoverContent className="w-80">
                    {toolItems.length > 0 ? (
                        <div className="space-y-2">
                            <TypographySmall className="font-semibold">
                                Available Tools
                            </TypographySmall>
                            <div className="space-y-1">
                                {toolItems.map(renderToolItem)}
                            </div>
                        </div>
                    ) : (
                        <TypographyMuted>No tools available</TypographyMuted>
                    )}
                </PopoverContent>
            </Popover>
        </Tooltip>
    );

    function renderToolItem({ isEnabled, isToggleable, tool }: ToolItem) {
        return (
            <ToolEntry
                key={tool}
                tool={tool}
                actions={
                    isToggleable ? (
                        <Switch
                            checked={isEnabled}
                            onCheckedChange={(checked) =>
                                handleToggleTool(tool, checked)
                            }
                        />
                    ) : (
                        <TypographyMuted>On</TypographyMuted>
                    )
                }
            />
        );
    }
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
        <Tooltip>
            <TooltipContent>Knowledge</TooltipContent>

            <Popover>
                <TooltipTrigger asChild>
                    <PopoverTrigger asChild>
                        <Button
                            size="icon-sm"
                            variant="outline"
                            className={cn("gap-2", className)}
                            startContent={<LibraryIcon />}
                            disabled={disabled}
                        />
                    </PopoverTrigger>
                </TooltipTrigger>

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
                        <TypographyMuted>
                            No knowledge available
                        </TypographyMuted>
                    )}
                </PopoverContent>
            </Popover>
        </Tooltip>
    );
}

function FilesInfo() {
    return (
        <Tooltip>
            <TooltipContent>Files</TooltipContent>

            <TooltipTrigger asChild>
                <Button
                    size="icon-sm"
                    variant="outline"
                    className="gap-2"
                    startContent={<PaperclipIcon />}
                />
            </TooltipTrigger>
        </Tooltip>
    );
}

function TablesInfo() {
    return (
        <Tooltip>
            <TooltipContent>Tables (Coming Soon)</TooltipContent>

            <TooltipTrigger asChild>
                <div>
                    <Button
                        size="icon-sm"
                        variant="outline"
                        className="gap-2"
                        startContent={<TableIcon />}
                        disabled
                    />
                </div>
            </TooltipTrigger>
        </Tooltip>
    );
}
