import { AlertTriangleIcon, PlusIcon } from "lucide-react";
import { useMemo, useState } from "react";
import useSWR from "swr";

import { OAuthProvider } from "~/lib/model/oauthApps/oauth-helpers";
import {
    ToolCategory,
    ToolReferenceService,
} from "~/lib/service/api/toolreferenceService";
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
import { useOAuthAppList } from "~/hooks/oauthApps/useOAuthApps";

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
    const [search, setSearch] = useState("");

    const oauthApps = useOAuthAppList();
    const configuredOauthApps = useMemo(() => {
        return new Set(
            oauthApps
                .filter((app) => !app.noGatewayIntegration)
                .map((app) => app.type)
        );
    }, [oauthApps]);

    const sortedValidCategories = useMemo(() => {
        return (
            Object.entries(toolCategories)
                .sort(([nameA, categoryA], [nameB, categoryB]): number => {
                    const aHasBundle = categoryA.bundleTool ? 1 : 0;
                    const bHasBundle = categoryB.bundleTool ? 1 : 0;

                    if (aHasBundle !== bHasBundle)
                        return bHasBundle - aHasBundle;

                    return nameA.localeCompare(nameB);
                })
                // filter out bundles with oauth providers that are not configured
                .filter(([, { bundleTool }]) => {
                    if (!bundleTool) return true;
                    const oauthType = bundleTool.metadata?.oauth;

                    return oauthType
                        ? configuredOauthApps.has(oauthType as OAuthProvider)
                        : true;
                })
        );
    }, [toolCategories, configuredOauthApps]);

    if (isLoading) return <LoadingSpinner />;

    const results = search.length
        ? filterToolCatalogBySearch(sortedValidCategories)
        : sortedValidCategories;
    return (
        <Command
            className={cn(
                "border w-full h-full",
                className,
                invert ? "flex-col-reverse" : "flex-col"
            )}
            shouldFilter={false}
        >
            <CommandInput
                placeholder="Search tools..."
                value={search}
                onValueChange={setSearch}
            />
            <div className="border-t shadow-2xl" />
            <CommandList className={cn("py-2 max-h-full", classNames?.list)}>
                <CommandEmpty>
                    <small className="flex items-center justify-center">
                        <AlertTriangleIcon className="w-4 h-4 mr-2" />
                        No results found.
                    </small>
                </CommandEmpty>
                {results.map(([category, categoryTools]) => (
                    <ToolCatalogGroup
                        key={category}
                        category={category}
                        tools={categoryTools}
                        selectedTools={tools}
                        onUpdateTools={onUpdateTools}
                    />
                ))}
            </CommandList>
        </Command>
    );

    function filterToolCatalogBySearch(
        toolCategories: [string, ToolCategory][]
    ) {
        return toolCategories.reduce(
            (acc, [category, categoryData]) => {
                const matchesSearch = (str: string) =>
                    str.toLowerCase().includes(search.toLowerCase());

                // Check if category name matches
                if (matchesSearch(category)) {
                    acc.push([category, categoryData]);
                    return acc;
                }

                // Check if bundle tool matches
                if (
                    categoryData.bundleTool &&
                    matchesSearch(categoryData.bundleTool.name)
                ) {
                    acc.push([category, categoryData]);
                    return acc;
                }

                // Filter tools and only include category if it has matching tools
                const filteredTools = categoryData.tools.filter(
                    (tool) =>
                        matchesSearch(tool.name ?? "") ||
                        matchesSearch(tool.description ?? "")
                );

                if (filteredTools.length > 0) {
                    acc.push([
                        category,
                        { ...categoryData, tools: filteredTools },
                    ]);
                }

                return acc;
            },
            [] as [string, ToolCategory][]
        );
    }
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
