import { useState } from "react";

import {
	ToolReference,
	UncategorizedToolCategory,
} from "~/lib/model/toolReferences";

import { ToolItem } from "~/components/tools/ToolItem";
import { CommandGroup } from "~/components/ui/command";

export function ToolCatalogGroup({
	category,
	configuredTools,
	tools,
	selectedTools,
	onAddTool,
	onRemoveTool,
}: {
	category: string;
	configuredTools: Set<string>;
	tools: ToolReference[];
	selectedTools: string[];
	onAddTool: (
		toolId: string,
		toolsToRemove: string[],
		oauthToAdd?: string
	) => void;
	onRemoveTool: (toolId: string, oauthToRemove?: string) => void;
	oauths: string[];
}) {
	const handleSelect = (
		toolId: string,
		bundleToolId: string,
		toolOauth?: string
	) => {
		if (selectedTools.includes(toolId)) {
			onRemoveTool(toolId, toolOauth);
		} else {
			onAddTool(toolId, [bundleToolId], toolOauth);
		}
	};

	const handleSelectBundle = (
		bundleToolId: string,
		bundleTool: ToolReference,
		toolOauth?: string
	) => {
		if (selectedTools.includes(bundleToolId)) {
			onRemoveTool(bundleToolId, toolOauth);
		} else {
			onAddTool(
				bundleToolId,
				bundleTool.tools?.map((tool) => tool.id) ?? [],
				toolOauth
			);
		}
	};

	const [expanded, setExpanded] = useState<Record<string, boolean>>({});

	return (
		<CommandGroup
			key={category}
			heading={category !== UncategorizedToolCategory ? category : undefined}
		>
			{tools.map((tool) => {
				const configured = configuredTools.has(tool.id);

				return (
					<>
						<ToolItem
							key={tool.id}
							tool={tool}
							configured={configured}
							isSelected={selectedTools.includes(tool.id)}
							isBundleSelected={false}
							onSelect={(toolOauthToAdd) =>
								handleSelectBundle(tool.id, tool, toolOauthToAdd)
							}
							expanded={expanded[tool.id]}
							onExpand={(expanded) => {
								setExpanded((prev) => ({
									...prev,
									[tool.id]: expanded,
								}));
							}}
							isBundle
						/>

						{expanded[tool.id] &&
							tool.tools?.map((categoryTool) => (
								<ToolItem
									key={categoryTool.id}
									configured={configured}
									tool={categoryTool}
									isSelected={selectedTools.includes(categoryTool.id)}
									isBundleSelected={selectedTools.includes(tool.id)}
									onSelect={(toolOauthToAdd) =>
										handleSelect(categoryTool.id, tool.id, toolOauthToAdd)
									}
								/>
							))}
					</>
				);
			})}
		</CommandGroup>
	);
}
