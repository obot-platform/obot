import { WrenchIcon } from "lucide-react";

import { ToolReference } from "~/lib/model/toolReferences";

import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
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
        <TooltipProvider>
            <Tooltip>
                <TooltipTrigger asChild>{children}</TooltipTrigger>
                <TooltipContent
                    sideOffset={isBundle ? 255 : 30}
                    side={isBundle ? "left" : "left"}
                    className="w-[300px] p-4 flex items-center"
                >
                    {tool.metadata?.icon ? (
                        <img
                            alt={tool.name}
                            src={tool.metadata.icon}
                            className="w-10 h-10 mr-4 dark:invert"
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
        </TooltipProvider>
    );
}
