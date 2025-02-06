import { useEffect, useState } from "react";

import { ToolCategory } from "~/lib/model/toolReferences";
import { cn } from "~/lib/utils";

import { ToolItem } from "~/components/tools/ToolItem";
import { CommandGroup } from "~/components/ui/command";

export function ToolCatalogGroup({
	category,
	configuredTools,
	tools,
	selectedTools,
	onAddTool,
	onRemoveTool,
	expandFor,
}: {
	category: string;
	configuredTools: Set<string>;
	tools: ToolCategory;
	selectedTools: string[];
	onAddTool: (
		toolId: string,
		toolsToRemove: string[],
		oauthToAdd?: string
	) => void;
	onRemoveTool: (toolId: string, oauthToRemove?: string) => void;
	oauths: string[];
	expandFor?: string;
}) {
	const handleSelect = (toolId: string, toolOauth?: string) => {
		if (selectedTools.includes(toolId)) {
			onRemoveTool(toolId, toolOauth);
		} else {
			onAddTool(
				toolId,
				tools.bundleTool?.id ? [tools.bundleTool.id] : [],
				toolOauth
			);
		}
	};

	const handleSelectBundle = (bundleToolId: string, toolOauth?: string) => {
		if (selectedTools.includes(bundleToolId)) {
			onRemoveTool(bundleToolId, toolOauth);
		} else {
			onAddTool(
				bundleToolId,
				tools.tools.map((tool) => tool.id),
				toolOauth
			);
		}
	};

	const [expanded, setExpanded] = useState(() => {
		const set = new Set(tools.tools.map((tool) => tool.id));
		return selectedTools.some((tool) => set.has(tool));
	});

	useEffect(() => {
		const containsMatchingTool =
			expandFor?.length &&
			tools.tools.some(
				(tool) =>
					tool.description?.toLowerCase().includes(expandFor) ||
					tool.name?.toLowerCase().includes(expandFor)
			);
		setExpanded(containsMatchingTool || false);
	}, [expandFor, tools]);

	return (
		<CommandGroup
			key={category}
			className={cn({
				"has-[.group-heading:hover]:bg-accent": !!tools.bundleTool,
			})}
			heading={!tools.bundleTool ? category : undefined}
		>
			{tools.bundleTool && (
				<ToolItem
					tool={tools.bundleTool}
					configured={configuredTools.has(tools.bundleTool.id)}
					isSelected={selectedTools.includes(tools.bundleTool.id)}
					isBundleSelected={false}
					onSelect={(toolOauthToAdd) =>
						handleSelectBundle(tools.bundleTool!.id, toolOauthToAdd)
					}
					expanded={expanded}
					onExpand={setExpanded}
					isBundle
				/>
			)}

			{(expanded || !tools.bundleTool) &&
				tools.tools.map((categoryTool) => (
					<ToolItem
						key={categoryTool.id}
						tool={categoryTool}
						configured={configuredTools.has(categoryTool.id)}
						isSelected={selectedTools.includes(categoryTool.id)}
						isBundleSelected={
							tools.bundleTool
								? selectedTools.includes(tools.bundleTool.id)
								: false
						}
						hideWarning={!!tools.bundleTool}
						onSelect={(toolOauthToAdd) =>
							handleSelect(categoryTool.id, toolOauthToAdd)
						}
					/>
				))}
		</CommandGroup>
	);
}
