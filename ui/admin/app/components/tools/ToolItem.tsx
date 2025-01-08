import { ToolReference } from "~/lib/model/toolReferences";
import { cn } from "~/lib/utils";

import { ToolIcon } from "~/components/tools/ToolIcon";
import { ToolTooltip } from "~/components/tools/ToolTooltip";
import { Button } from "~/components/ui/button";
import { Checkbox } from "~/components/ui/checkbox";
import { CommandItem } from "~/components/ui/command";

type ToolItemProps = {
    tool: ToolReference;
    isSelected: boolean;
    isBundleSelected: boolean;
    onSelect: () => void;
    expanded?: boolean;
    onExpand?: (expanded: boolean) => void;
    className?: string;
    isBundle?: boolean;
};

export function ToolItem({
    tool,
    isSelected,
    isBundleSelected,
    onSelect,
    expanded,
    onExpand,
    className,
    isBundle,
}: ToolItemProps) {
    return (
        <CommandItem
            className={cn("cursor-pointer", className)}
            onSelect={onSelect}
            disabled={isBundleSelected}
        >
            <ToolTooltip tool={tool}>
                <div
                    className={cn(
                        "flex justify-between items-center w-full gap-2"
                    )}
                >
                    <span
                        className={cn(
                            "text-sm font-medium flex items-center w-full gap-2 px-4",
                            {
                                "px-0": isBundle,
                            }
                        )}
                    >
                        <Checkbox checked={isSelected || isBundleSelected} />

                        <span className={cn("flex items-center")}>
                            <ToolIcon
                                icon={tool.metadata?.icon}
                                category={tool.metadata?.category}
                                name={tool.name}
                                className="w-4 h-4 mr-2"
                                disableTooltip
                            />
                            {tool.name}
                        </span>
                    </span>

                    {isBundle && (
                        <Button
                            variant="link"
                            size="link-sm"
                            onClick={(e) => {
                                e.stopPropagation();
                                onExpand?.(!expanded);
                            }}
                        >
                            {expanded ? "Show Less" : "Show More"}
                        </Button>
                    )}
                </div>
            </ToolTooltip>
        </CommandItem>
    );
}
