import { useEffect, useState } from "react";

import { ToolCategory } from "~/lib/service/api/toolreferenceService";
import { cn } from "~/lib/utils";

import { ToolItem } from "~/components/tools/ToolItem";
import { CommandGroup } from "~/components/ui/command";

export function ToolCatalogGroup({
	category,
	tools,
	selectedTools,
	onUpdateTools,
	expandFor,
}: {
	category: string;
	tools: ToolCategory;
	selectedTools: string[];
	onUpdateTools: (tools: string[]) => void;
	expandFor?: string;
}) {
	const handleSelect = (toolId: string) => {
		if (selectedTools.includes(toolId)) {
			onUpdateTools(selectedTools.filter((tool) => tool !== toolId));
		}

		const newTools = selectedTools
			.filter((tool) => tool !== tools.bundleTool?.id)
			.concat(toolId);

		onUpdateTools(newTools);
	};

	const handleSelectBundle = (bundleToolId: string) => {
		if (selectedTools.includes(bundleToolId)) {
			onUpdateTools(selectedTools.filter((tool) => tool !== bundleToolId));
			return;
		}

		const toolsToRemove = new Set(tools.tools.map((tool) => tool.id));

		const newTools = [
			...selectedTools.filter((tool) => !toolsToRemove.has(tool)),
			bundleToolId,
		];

		onUpdateTools(newTools);
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
					isSelected={selectedTools.includes(tools.bundleTool.id)}
					isBundleSelected={false}
					onSelect={() => handleSelectBundle(tools.bundleTool!.id)}
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
						isSelected={selectedTools.includes(categoryTool.id)}
						isBundleSelected={
							tools.bundleTool
								? selectedTools.includes(tools.bundleTool.id)
								: false
						}
						onSelect={() => handleSelect(categoryTool.id)}
					/>
				))}
		</CommandGroup>
	);
}
