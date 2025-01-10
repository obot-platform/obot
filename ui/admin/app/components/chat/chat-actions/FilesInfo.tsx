import { PaperclipIcon } from "lucide-react";

import { Button } from "~/components/ui/button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";

export function FilesInfo() {
	return (
		<Tooltip>
			<TooltipContent>Files (Coming Soon)</TooltipContent>

			<TooltipTrigger asChild>
				<div>
					<Button
						size="icon-sm"
						variant="outline"
						className="gap-2"
						startContent={<PaperclipIcon />}
						disabled
					/>
				</div>
			</TooltipTrigger>
		</Tooltip>
	);
}
