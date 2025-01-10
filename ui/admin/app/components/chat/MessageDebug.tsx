import { CodeIcon, InfoIcon, Maximize2Icon, Minimize2Icon } from "lucide-react";
import { useState } from "react";

import { Calls } from "~/lib/model/runs";
import { RunsService } from "~/lib/service/api/runsService";

import CallFrames from "~/components/chat/CallFrames";
import { Button } from "~/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from "~/components/ui/dialog";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";

type MessageDebugProps = {
	runId: string;
};

export function MessageDebug({ runId }: MessageDebugProps) {
	const [runDebug, setRunDebug] = useState<Calls>({});
	const [isFullscreen, setIsFullscreen] = useState(false);

	return (
		<Dialog>
			<Tooltip>
				<TooltipTrigger asChild>
					<DialogTrigger asChild>
						<Button
							size="icon"
							variant="ghost"
							onClick={() => {
								RunsService.getRunDebugById(runId).then((runDebug) => {
									setRunDebug(runDebug.frames);
								});
							}}
						>
							<InfoIcon className="h-4 w-4" />
						</Button>
					</DialogTrigger>
				</TooltipTrigger>
				<TooltipContent>Debug Information</TooltipContent>
			</Tooltip>

			<DialogContent
				className={`transition-all duration-300 ease-in-out ${
					isFullscreen
						? "max-w-screen h-screen max-h-screen w-screen !rounded-none !p-6"
						: "h-[80vh] w-[50vw] max-w-[80vw]"
				} flex flex-col`}
			>
				<DialogHeader>
					<DialogTitle>
						<div className="flex items-center gap-2">
							<CodeIcon className="h-4 w-4" />
							Run {runId}
						</div>
						<Button
							variant="ghost"
							className="absolute right-10 top-2 text-muted-foreground"
							size="icon"
							onClick={() => setIsFullscreen(!isFullscreen)}
						>
							{isFullscreen ? (
								<Minimize2Icon className="h-3.5 w-3.5" />
							) : (
								<Maximize2Icon className="h-3.5 w-3.5" />
							)}
						</Button>
					</DialogTitle>
				</DialogHeader>
				<DialogDescription>
					Click below to see more information about what took place behind the
					scenes for this particular message.
				</DialogDescription>
				{runDebug && <CallFrames calls={runDebug} />}
			</DialogContent>
		</Dialog>
	);
}
