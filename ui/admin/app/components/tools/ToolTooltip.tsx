import { WrenchIcon } from "lucide-react";

import { ToolReference } from "~/lib/model/toolReferences";

import { ToolIcon } from "~/components/tools/ToolIcon";
import {
    Tooltip,
    TooltipContent,
    TooltipTrigger,
} from "~/components/ui/tooltip";

type ToolTooltipProps = {
    tool: ToolReference;
    children: React.ReactNode;
    isBundle?: boolean;
};

export function ToolTooltip({
    tool,
    children,
    isBundle = false,
}: ToolTooltipProps) {
    return (
        <Tooltip>
            <TooltipTrigger asChild>{children}</TooltipTrigger>
            <TooltipContent
                sideOffset={isBundle ? 255 : 30}
                side={isBundle ? "left" : "left"}
                className="w-[300px] p-4 flex items-center bg-background text-foreground border"
            >
                {tool.metadata?.icon ? (
                    <ToolIcon
                        icon={tool.metadata?.icon}
                        category={tool.metadata?.category}
                        name={tool.name}
                        className="w-10 h-10 mr-4"
                        disableTooltip
                    />
                ) : (
                    <WrenchIcon className="w-4 h-4 mr-2" />
                )}
                <div>
                    <p className="font-bold">
                        {tool.name}
                        {isBundle ? " Bundle" : ""}
                    </p>
                    <p className="text-sm">
                        {tool.description || "No description provided."}
                    </p>
                </div>
            </TooltipContent>
        </Tooltip>
    );
}
