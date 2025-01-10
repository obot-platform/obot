import { Folder } from "lucide-react";

import { ToolReference } from "~/lib/model/toolReferences";

import { ToolIcon } from "~/components/tools/ToolIcon";
import { Badge } from "~/components/ui/badge";

interface CategoryHeaderProps {
	category: string;
	description: string;
	tools: ToolReference[];
}

export function CategoryHeader({
	category,
	tools,
	description,
}: CategoryHeaderProps) {
	return (
		<div className="flex items-center space-x-4">
			<div className="mb-2 flex h-10 w-10 items-center justify-center rounded-full border">
				{tools[0]?.metadata?.icon ? (
					<ToolIcon
						className="h-6 w-6"
						name={description}
						icon={tools[0].metadata.icon}
						disableTooltip={!description}
					/>
				) : (
					<Folder className="h-6 w-6" />
				)}
			</div>
			<h2 className="flex items-center space-x-2">
				<span>{category}</span>
				<Badge className="pointer-events-none">{tools.length}</Badge>
			</h2>
		</div>
	);
}
