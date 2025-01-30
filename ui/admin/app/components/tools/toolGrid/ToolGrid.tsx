import { useMemo } from "react";

import {
	CustomToolsToolCategory,
	ToolCategory,
	ToolReference,
} from "~/lib/model/toolReferences";

import { BundleToolList } from "~/components/tools/toolGrid/BundleToolList";
import { ToolCard } from "~/components/tools/toolGrid/ToolCard";

export function ToolGrid({
	toolCategories,
}: {
	toolCategories: [string, ToolCategory][];
}) {
	const { customTools, builtinTools } = useMemo(() => {
		return separateCustomAndBuiltinTools(toolCategories);
	}, [toolCategories]);

	const sortedCustomTools =
		customTools.sort((a, b) => {
			// Sort by created descending for custom tools
			const aCreatedAt =
				"bundleTool" in a
					? a.bundleTool?.created
					: (a as ToolReference).created;
			const bCreatedAt =
				"bundleTool" in b
					? b.bundleTool?.created
					: (b as ToolReference).created;

			return (
				new Date(bCreatedAt ?? "").getTime() -
				new Date(aCreatedAt ?? "").getTime()
			);
		}) ?? [];

	const sortedBuiltinTools = builtinTools.sort((a, b) => {
		const aName =
			"bundleTool" in a ? a.bundleTool?.name : (a as ToolReference).name;
		const bName =
			"bundleTool" in b ? b.bundleTool?.name : (b as ToolReference).name;
		return (aName ?? "").localeCompare(bName ?? "");
	});

	return (
		<div className="flex flex-col gap-8">
			{sortedCustomTools.length > 0 && (
				<div className="flex flex-col gap-4">
					<h3>{CustomToolsToolCategory}</h3>
					<div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
						{sortedCustomTools.map(renderToolCard)}
					</div>
				</div>
			)}

			{sortedBuiltinTools.length > 0 && (
				<div className="flex flex-col gap-4">
					<h3>Built-in Tools</h3>
					<div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
						{sortedBuiltinTools.map(renderToolCard)}
					</div>
				</div>
			)}
		</div>
	);

	function renderToolCard(item: ToolCategory | ToolReference) {
		if ("bundleTool" in item && item.bundleTool) {
			return (
				<ToolCard
					key={item.bundleTool.id}
					HeaderRightContent={
						item.tools.length > 0 ? (
							<BundleToolList tools={item.tools} bundle={item.bundleTool} />
						) : null
					}
					tool={item.bundleTool}
				/>
			);
		}

		if ("name" in item) return <ToolCard key={item.name} tool={item} />;

		return null;
	}

	function separateCustomAndBuiltinTools(
		toolCategories: [string, ToolCategory][]
	) {
		return toolCategories.reduce<{
			customTools: (ToolCategory | ToolReference)[];
			builtinTools: (ToolCategory | ToolReference)[];
		}>(
			(acc, [, { bundleTool, tools }]) => {
				if (bundleTool) {
					const key = bundleTool.builtin ? "builtinTools" : "customTools";
					acc[key].push({ bundleTool, tools });
				} else {
					tools.forEach((tool) => {
						if (tool.builtin) {
							acc.builtinTools.push(tool);
						} else {
							acc.customTools.push(tool);
						}
					});
				}
				return acc;
			},
			{ customTools: [], builtinTools: [] }
		);
	}
}
