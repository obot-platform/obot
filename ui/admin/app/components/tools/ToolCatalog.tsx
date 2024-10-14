import { AlertTriangleIcon } from "lucide-react";
import { useCallback } from "react";
import useSWR from "swr";

import { ToolReference } from "~/lib/model/toolReferences";
import { ToolReferenceService } from "~/lib/service/api/toolreferenceService";
import { cn } from "~/lib/utils";

import { ToolItem } from "~/components/tools/ToolItem";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import {
    Command,
    CommandEmpty,
    CommandGroup,
    CommandInput,
    CommandList,
} from "~/components/ui/command";

import { ToolCategoryHeader } from "./ToolCategoryHeader";

type ToolCatalogProps = React.HTMLAttributes<HTMLDivElement> & {
    tools: string[];
    onChangeTools: (tools: string[]) => void;
    invert?: boolean;
};

export function ToolCatalog({
    className,
    tools,
    invert = false,
    onChangeTools,
}: ToolCatalogProps) {
    const { data: toolCategories = [], isLoading } = useSWR(
        ToolReferenceService.getToolReferencesCategoryMap.key("tool"),
        () => ToolReferenceService.getToolReferencesCategoryMap("tool"),
        { fallbackData: {} }
    );

    const handleSelect = useCallback(
        (toolId: string) => {
            if (!tools.includes(toolId)) {
                onChangeTools([...tools, toolId]);
            }
        },
        [tools, onChangeTools]
    );

    const handleSelectBundle = useCallback(
        (bundleToolId: string, categoryTools: ToolReference[]) => {
            const categoryToolIds = categoryTools.map((tool) => tool.id);
            const newTools = tools.includes(bundleToolId)
                ? tools.filter((toolId) => toolId !== bundleToolId)
                : [
                      ...tools.filter(
                          (toolId) => !categoryToolIds.includes(toolId)
                      ),
                      bundleToolId,
                  ];
            onChangeTools(newTools);
        },
        [tools, onChangeTools]
    );

    if (isLoading) return <LoadingSpinner />;

    return (
        <Command
            className={cn(
                "border w-[300px] px-2",
                className,
                invert ? "flex-col-reverse" : "flex-col"
            )}
            filter={(value, search, keywords) => {
                return value.toLowerCase().includes(search.toLowerCase()) ||
                    keywords?.some((keyword) =>
                        keyword.toLowerCase().includes(search.toLowerCase())
                    )
                    ? 1
                    : 0;
            }}
        >
            <CommandInput placeholder="Search tools..." />
            <div className="border-t shadow-2xl" />
            <CommandList className="py-2">
                <CommandEmpty>
                    <h1 className="flex items-center justify-center">
                        <AlertTriangleIcon className="w-4 h-4 mr-2" />
                        No results found.
                    </h1>
                </CommandEmpty>
                {Object.entries(toolCategories).map(
                    ([category, categoryTools]) => (
                        <CommandGroup
                            key={category}
                            heading={
                                <ToolCategoryHeader
                                    category={category}
                                    categoryTools={categoryTools}
                                    tools={tools}
                                    onSelectBundle={handleSelectBundle}
                                />
                            }
                        >
                            {categoryTools.tools.map((categoryTool) => (
                                <ToolItem
                                    key={categoryTool.id}
                                    tool={categoryTool}
                                    isSelected={tools.includes(categoryTool.id)}
                                    isBundleSelected={
                                        categoryTools.bundleTool
                                            ? tools.includes(
                                                  categoryTools.bundleTool.id
                                              )
                                            : false
                                    }
                                    onSelect={() =>
                                        handleSelect(categoryTool.id)
                                    }
                                />
                            ))}
                        </CommandGroup>
                    )
                )}
            </CommandList>
        </Command>
    );
}
