import { PlusCircle } from "lucide-react";

import { ToolReference } from "~/lib/model/toolReferences";

import { Button } from "~/components/ui/button";
import { Card } from "~/components/ui/card";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";

export function StepTemplateCard({
	stepTemplate,
	onClick,
}: {
	stepTemplate: ToolReference;
	onClick: () => void;
}) {
	return (
		<Card className="my-2 flex items-center justify-between space-x-4 truncate p-4">
			<div className="truncate text-sm">
				<h1 className="truncate">{stepTemplate.name}</h1>
				<h2 className="truncate text-gray-500">{stepTemplate.description}</h2>
			</div>

			<Tooltip>
				<TooltipContent>Add template</TooltipContent>
				<TooltipTrigger>
					<Button onClick={onClick} size="icon" variant="secondary">
						<PlusCircle />
					</Button>
				</TooltipTrigger>
			</Tooltip>
		</Card>
	);
}
