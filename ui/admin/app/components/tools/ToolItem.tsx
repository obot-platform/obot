import { ToolReference } from "~/lib/model/toolReferences";

import { ToolIcon } from "~/components/tools/ToolIcon";
import { ToolTooltip } from "~/components/tools/ToolTooltip";
import { CommandItem } from "~/components/ui/command";

type ToolItemProps = {
    tool: ToolReference;
    isSelected: boolean;
    isBundleSelected: boolean;
    onSelect: () => void;
};

export function ToolItem({
    tool,
    isSelected,
    isBundleSelected,
    onSelect,
}: ToolItemProps) {
    return (
        <CommandItem
            className="cursor-pointer"
            keywords={[
                tool.description || "",
                tool.name || "",
                tool.metadata?.category || "",
            ]}
            onSelect={onSelect}
            disabled={isSelected || isBundleSelected}
        >
            <ToolTooltip tool={tool}>
                <span className="text-sm font-medium flex items-center w-full px-2">
                    <ToolIcon
                        icon={tool.metadata?.icon}
                        category={tool.metadata?.category}
                        name={tool.name}
                        className="w-4 h-4 mr-2"
                        disableTooltip
                    />
                    {tool.name}
                </span>
            </ToolTooltip>
        </CommandItem>
    );
}
