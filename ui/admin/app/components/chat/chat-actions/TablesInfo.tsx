import { TableIcon } from "lucide-react";

import { Button } from "~/components/ui/button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";

export function TablesInfo() {
	return (
		<Tooltip>
			<TooltipContent>Tables (Coming Soon)</TooltipContent>

			<TooltipTrigger asChild>
				<div>
					<Button
						size="icon-sm"
						variant="outline"
						className="gap-2"
						startContent={<TableIcon />}
						disabled
					/>
				</div>
			</TooltipTrigger>
		</Tooltip>
	);
}
