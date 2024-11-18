import { AlertTriangleIcon, PlusIcon } from "lucide-react";
import { useCallback } from "react";
import useSWR from "swr";

import { ToolReference } from "~/lib/model/toolReferences";
import { ToolReferenceService } from "~/lib/service/api/toolreferenceService";
import { cn } from "~/lib/utils";

import { ToolCategoryHeader } from "~/components/tools/ToolCategoryHeader";
import { ToolItem } from "~/components/tools/ToolItem";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Button } from "~/components/ui/button";
import {
    Command,
    CommandEmpty,
    CommandGroup,
    CommandInput,
    CommandList,
} from "~/components/ui/command";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogTitle,
    DialogTrigger,
} from "~/components/ui/dialog";

type ToolCatalogProps = React.HTMLAttributes<HTMLDivElement> & {
    tools: string[];
    onAddTool: (tools: string) => void;
    onRemoveTools: (tools: string[]) => void;
    invert?: boolean;
    classNames?: { list?: string };
};

export function ToolCatalog({
    className,
    tools,
    invert = false,
    onAddTool,
    onRemoveTools,
    classNames,
}: ToolCatalogProps) {
    const { data: toolCategories, isLoading } = useSWR(
        ToolReferenceService.getToolReferencesCategoryMap.key("tool"),
        () => ToolReferenceService.getToolReferencesCategoryMap("tool"),
        { fallbackData: {} }
    );

    const handleSelect = useCallback(
        (toolId: string) => {
            if (!tools.includes(toolId)) {
                onAddTool(toolId);
            }
        },
        [tools, onAddTool]
    );

    const handleSelectBundle = useCallback(
        (bundleToolId: string, categoryTools: ToolReference[]) => {
            if (tools.includes(bundleToolId)) {
                onRemoveTools([bundleToolId]);
                return;
            }

            onAddTool(bundleToolId);

            // remove all tools in the bundle to remove redundancy
            onRemoveTools(categoryTools.map((tool) => tool.id));
        },
        [tools, onAddTool, onRemoveTools]
    );

    if (isLoading) return <LoadingSpinner />;

    return (
        <Command
            className={cn(
                "border w-full h-full",
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
            <CommandList className={cn("py-2 max-h-full", classNames?.list)}>
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

export function ToolCatalogDialog(props: ToolCatalogProps) {
    return (
        <Dialog>
            <DialogContent className="p-0 h-[60vh]">
                <DialogTitle hidden>Tool Catalog</DialogTitle>
                <DialogDescription hidden>
                    Add tools to the agent.
                </DialogDescription>
                <ToolCatalog {...props} />
            </DialogContent>

            <DialogTrigger asChild>
                <Button variant="ghost">
                    <PlusIcon className="w-4 h-4 mr-2" /> Add Tool
                </Button>
            </DialogTrigger>
        </Dialog>
    );
}
