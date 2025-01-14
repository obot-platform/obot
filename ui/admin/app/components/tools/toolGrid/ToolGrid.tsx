import {
	CustomToolsToolCategory,
	ToolCategory,
} from "~/lib/model/toolReferences";

import { BundleToolList } from "~/components/tools/toolGrid/BundleToolList";
import { ToolCard } from "~/components/tools/toolGrid/ToolCard";

export function ToolGrid({
	toolCategories,
}: {
	toolCategories: [string, ToolCategory][];
}) {
	const sortedCustomTools =
		toolCategories
			.find(([category]) => category === CustomToolsToolCategory)?.[1]
			.tools?.sort((a, b) => {
				// sort by created date descending
				return new Date(b.created).getTime() - new Date(a.created).getTime();
			}) ?? [];

	const sortedBuiltinTools = toolCategories
		.filter(([category]) => category !== CustomToolsToolCategory)
		.sort((a, b) => {
			return a[0].localeCompare(b[0]);
		});

	return (
		<div className="flex flex-col gap-8">
			{sortedCustomTools.length > 0 && (
				<div className="flex flex-col gap-4">
					<h3>Custom Tools</h3>
					<div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
						{sortedCustomTools.map((tool) => (
							<ToolCard key={tool.id} tool={tool} />
						))}
					</div>
				</div>
			)}

			{sortedBuiltinTools.length > 0 && (
				<div className="flex flex-col gap-4">
					<h3>Built-in Tools</h3>
					<div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
						{sortedBuiltinTools.map(([, { tools, bundleTool }]) => {
							if (bundleTool) {
								return (
									<ToolCard
										key={bundleTool.id}
										HeaderRightContent={
											tools.length > 0 ? (
												<BundleToolList tools={tools} bundle={bundleTool} />
											) : null
										}
										tool={bundleTool}
									/>
								);
							}
							return tools.map((tool) => (
								<ToolCard key={tool.id} tool={tool} />
							));
						})}
					</div>
				</div>
			)}
		</div>
	);
}
