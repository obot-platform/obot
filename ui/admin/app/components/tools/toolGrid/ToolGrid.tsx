import { useMemo } from "react";

import {
	CustomToolsToolCategory,
	ToolReference,
	isCapabilityTool,
} from "~/lib/model/toolReferences";

import { BundleToolList } from "~/components/tools/toolGrid/BundleToolList";
import { ToolCard } from "~/components/tools/toolGrid/ToolCard";

export function ToolGrid({ toolMap }: { toolMap: [string, ToolReference][] }) {
	const { customTools, builtinTools, capabilities } = useMemo(() => {
		return separateCustomAndBuiltinTools(toolMap);
	}, [toolMap]);

	const sortedCustomTools =
		customTools.sort((a, b) => {
			// Sort by created descending for custom tools
			const aCreatedAt = a.created;
			const bCreatedAt = b.created;

			return (
				new Date(bCreatedAt ?? "").getTime() -
				new Date(aCreatedAt ?? "").getTime()
			);
		}) ?? [];

	const sortedBuiltinTools = builtinTools.sort((a, b) => {
		const aName = a.name;
		const bName = b.name;
		return (aName ?? "").localeCompare(bName ?? "");
	});

	return (
		<div className="flex flex-col gap-8">
			{sortedCustomTools.length > 0 && (
				<div className="flex flex-col gap-4">
					<h3>{CustomToolsToolCategory}</h3>
					<div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
						{sortedCustomTools.map((tool) => renderToolCard(tool))}
					</div>
				</div>
			)}

			{sortedBuiltinTools.length > 0 && (
				<div className="flex flex-col gap-4">
					<h3>Built-in Tools</h3>
					<div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
						{sortedBuiltinTools.map((tool) => renderToolCard(tool))}
					</div>
				</div>
			)}

			{capabilities.length > 0 && (
				<div className="flex flex-col gap-4">
					<h3>Capabilities</h3>
					<div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
						{capabilities.map(renderToolCard)}
					</div>
				</div>
			)}
		</div>
	);

	function renderToolCard(item: ToolReference) {
		return (
			<ToolCard
				key={item.id}
				HeaderRightContent={
					item.tools && item.tools.length > 0 ? (
						<BundleToolList tools={item.tools} bundle={item} />
					) : null
				}
				tool={item}
			/>
		);
	}

	function separateCustomAndBuiltinTools(toolMap: [string, ToolReference][]) {
		return toolMap.reduce<{
			customTools: ToolReference[];
			builtinTools: ToolReference[];
			capabilities: ToolReference[];
		}>(
			(acc, [_, tool]) => {
				if (isCapabilityTool(tool)) {
					acc.capabilities.push(tool);
				} else if (tool.builtin) {
					acc.builtinTools.push(tool);
				} else {
					acc.customTools.push(tool);
				}
				return acc;
			},
			{ customTools: [], builtinTools: [], capabilities: [] }
		);
	}
}
