import { CheckIcon } from "lucide-react";

import { ToolReference } from "~/lib/model/toolReferences";
import { ToolCategory } from "~/lib/service/api/toolreferenceService";

import { ToolTooltip } from "~/components/tools/ToolTooltip";
import { Switch } from "~/components/ui/switch";

type ToolCategoryHeaderProps = {
    category: string;
    categoryTools: ToolCategory;
    tools: string[];
    onSelectBundle: (
        bundleToolId: string,
        categoryTools: ToolReference[]
    ) => void;
};

export function ToolCategoryHeader({
    category,
    categoryTools,
    tools,
    onSelectBundle,
}: ToolCategoryHeaderProps) {
    return (
        <div className="flex justify-between items-center w-full">
            {!categoryTools.bundleTool ? (
                <span>{category}</span>
            ) : (
                <>
                    <span className="flex items-center">
                        {category}
                        {tools.includes(categoryTools.bundleTool!.id) && (
                            <CheckIcon className="ml-2 h-4 w-4" />
                        )}
                    </span>
                    <ToolTooltip tool={categoryTools.bundleTool} isBundle>
                        <div className="flex items-center space-x-2">
                            <Switch
                                className="scale-75"
                                checked={tools.includes(
                                    categoryTools.bundleTool.id
                                )}
                                onCheckedChange={() =>
                                    onSelectBundle(
                                        categoryTools.bundleTool!.id,
                                        categoryTools.tools
                                    )
                                }
                            />
                        </div>
                    </ToolTooltip>
                </>
            )}
        </div>
    );
}
