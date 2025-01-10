import { ToolReference } from "~/lib/model/toolReferences";

import { ToolCard } from "~/components/tools/toolGrid/ToolCard";

interface CategoryToolsProps {
	tools: ToolReference[];
	onDelete: (id: string) => void;
}

export function CategoryTools({ tools, onDelete }: CategoryToolsProps) {
	return (
		<div className="grid grid-cols-1 gap-4 sm:grid-cols-1 md:grid-cols-2 lg:grid-cols-3">
			{tools.map((tool, index) => (
				<ToolCard key={`${tool.id}-${index}`} tool={tool} onDelete={onDelete} />
			))}
		</div>
	);
}
