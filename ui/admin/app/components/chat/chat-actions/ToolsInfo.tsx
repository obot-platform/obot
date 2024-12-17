import { WrenchIcon } from "lucide-react";
import { useMemo } from "react";

import { Agent } from "~/lib/model/agents";
import { cn } from "~/lib/utils";

import { TypographyMuted, TypographySmall } from "~/components/Typography";
import { ToolEntry } from "~/components/agent/ToolEntry";
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

type ToolItem = {
    tool: string;
    isToggleable: boolean;
    isEnabled: boolean;
};

export function ToolsInfo({
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

                <PopoverContent className="w-80" align="start">
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
