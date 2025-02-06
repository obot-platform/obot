import { AlertTriangleIcon, PlusIcon } from "lucide-react";
import { useMemo, useState } from "react";
import useSWR from "swr";

import {
	ToolCategory,
	convertToolReferencesToCategoryMap,
} from "~/lib/model/toolReferences";
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
import { useOauthAppMap } from "~/hooks/oauthApps/useOAuthApps";

type ToolCatalogProps = React.HTMLAttributes<HTMLDivElement> & {
	tools: string[];
	oauths: string[];
	onUpdateTools: (tools: string[], toolOauths: string[]) => void;
	invert?: boolean;
	classNames?: { list?: string };
};

export function ToolCatalog({
	className,
	tools: selectedTools,
	oauths,
	onUpdateTools,
	invert = false,
	classNames,
}: ToolCatalogProps) {
	const { data: toolList, isLoading } = useSWR(
		ToolReferenceService.getToolReferences.key("tool"),
		() => ToolReferenceService.getToolReferences("tool"),
		{ fallbackData: [] }
	);

	const toolCategories = useMemo(
		() => convertToolReferencesToCategoryMap(toolList),
		[toolList]
	);

	const oauthToolMap = useMemo(
		() => new Map(toolList.map((tool) => [tool.id, tool.metadata?.oauth])),
		[toolList]
	);

	const [search, setSearch] = useState("");
	const configuredOauthApps = useOauthAppMap();
	const configuredTools = useMemo(() => {
		return new Set(
			toolList
				.filter((tool) => {
					const oauth = tool.metadata?.oauth;
					return oauth ? configuredOauthApps.has(oauth) : true;
				})
				.map((tool) => tool.id)
		);
	}, [toolList, configuredOauthApps]);

	const sortedValidCategories = useMemo(() => {
		return Object.entries(toolCategories).sort(
			([nameA, categoryA], [nameB, categoryB]): number => {
				const aHasBundle = categoryA.bundleTool ? 1 : 0;
				const bHasBundle = categoryB.bundleTool ? 1 : 0;

				if (aHasBundle !== bHasBundle) return bHasBundle - aHasBundle;

				return nameA.localeCompare(nameB);
			}
		);
	}, [toolCategories]);

	if (isLoading) return <LoadingSpinner />;

	const results = search.length
		? filterToolCatalogBySearch(sortedValidCategories, search)
		: sortedValidCategories;

	const handleRemoveTool = (toolId: string, oauthToRemove?: string) => {
		const updatedTools = selectedTools.filter((tool) => tool !== toolId);
		const stillHasOauth = updatedTools.some(
			(tool) => oauthToolMap.get(tool) === oauthToRemove
		);
		const updatedOauths = stillHasOauth
			? oauths
			: oauths.filter((oauth) => oauth !== oauthToRemove);
		onUpdateTools(updatedTools, updatedOauths);
	};

	const handleAddTool = (
		toolId: string,
		toolsToRemove: string[],
		oauthToAdd?: string
	) => {
		const toolsToRemoveSet = new Set(toolsToRemove);
		const newTools = [
			...selectedTools.filter((tool) => !toolsToRemoveSet.has(tool)),
			toolId,
		];

		const updatedOauths =
			oauthToAdd && !oauths.includes(oauthToAdd)
				? [...oauths, oauthToAdd]
				: oauths;

		onUpdateTools(newTools, updatedOauths);
	};

	return (
		<Command
			className={cn(
				"h-full w-full border",
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
			<CommandList className={cn("max-h-full py-2", classNames?.list)}>
				<CommandEmpty>
					<small className="flex items-center justify-center">
						<AlertTriangleIcon className="mr-2 h-4 w-4" />
						No results found.
					</small>
				</CommandEmpty>
				{results.map(([category, categoryTools]) => (
					<ToolCatalogGroup
						key={category}
						category={category}
						tools={categoryTools}
						selectedTools={selectedTools}
						onAddTool={handleAddTool}
						onRemoveTool={handleRemoveTool}
						expandFor={search}
						oauths={oauths}
						configuredTools={configuredTools}
					/>
				))}
			</CommandList>
		</Command>
	);
}

export function ToolCatalogDialog(props: ToolCatalogProps) {
	return (
		<Dialog>
			<DialogContent className="h-[60vh] p-0">
				<DialogTitle hidden>Tool Catalog</DialogTitle>
				<DialogDescription hidden>Add tools to the agent.</DialogDescription>
				<ToolCatalog {...props} />
			</DialogContent>

			<DialogTrigger asChild>
				<Button variant="ghost">
					<PlusIcon /> Add Tools
				</Button>
			</DialogTrigger>
		</Dialog>
	);
}

export function filterToolCatalogBySearch(
	toolCategories: [string, ToolCategory][],
	query: string
) {
	return toolCategories.reduce<[string, ToolCategory][]>(
		(acc, [category, categoryData]) => {
			const matchesSearch = (str: string) =>
				str.toLowerCase().includes(query.toLowerCase());

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
				acc.push([category, { ...categoryData, tools: filteredTools }]);
			}

			return acc;
		},
		[]
	);
}
