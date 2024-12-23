import { AlertTriangleIcon, PlusIcon } from "lucide-react";
import useSWR from "swr";

import { ToolReferenceService } from "~/lib/service/api/toolreferenceService";
import { cn } from "~/lib/utils";

import { ToolCatalogGroup } from "~/components/tools/ToolCatalogGroup";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Button } from "~/components/ui/button";
import {
    Command,
    CommandEmpty,
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
    onUpdateTools: (tools: string[]) => void;
    invert?: boolean;
    classNames?: { list?: string };
};

export function ToolCatalog({
    className,
    tools,
    invert = false,
    onUpdateTools,
    classNames,
}: ToolCatalogProps) {
    const { data: toolCategories, isLoading } = useSWR(
        ToolReferenceService.getToolReferencesCategoryMap.key("tool"),
        () => ToolReferenceService.getToolReferencesCategoryMap("tool"),
        { fallbackData: {} }
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
                    </h1>{" "}
                </CommandEmpty>
                {Object.entries(toolCategories).map(
                    ([category, categoryTools]) => (
                        <ToolCatalogGroup
                            key={category}
                            category={category}
                            tools={categoryTools}
                            selectedTools={tools}
                            onUpdateTools={onUpdateTools}
                        />
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
