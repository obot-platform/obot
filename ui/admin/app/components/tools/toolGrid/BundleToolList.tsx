import { ToolReference } from "~/lib/model/toolReferences";

import { ToolIcon } from "~/components/tools/ToolIcon";
import { Badge } from "~/components/ui/badge";
import { Button } from "~/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from "~/components/ui/dialog";
import { ScrollArea } from "~/components/ui/scroll-area";
import { Separator } from "~/components/ui/separator";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";

type BundleToolListProps = {
	bundle: ToolReference;
	tools: ToolReference[];
	title?: string;
};

export function BundleToolList({
	bundle,
	tools,
	title = "Bundle",
}: BundleToolListProps) {
	return (
		<Dialog>
			<Tooltip>
				<TooltipTrigger asChild>
					<DialogTrigger asChild>
						<Button
							size="badge"
							className="mt-0 overflow-hidden py-0"
							classNames={{
								content: "gap-0",
							}}
							variant="secondary"
							endContent={
								<Badge className="relative left-2 min-w-6 justify-center rounded-none px-1">
									{tools.length}
								</Badge>
							}
						>
							{title}
						</Button>
					</DialogTrigger>
				</TooltipTrigger>
				<TooltipContent>View {bundle.name} Bundle</TooltipContent>
			</Tooltip>
			<DialogContent className="gap-0 p-0">
				<DialogHeader className="px-6 py-4">
					<DialogTitle className="flex items-center gap-2">
						{bundle.name} {title}
					</DialogTitle>
				</DialogHeader>

				<ScrollArea className="max-h-[40vh]">
					<div className="flex flex-col gap-4 px-6 pb-6">
						<DialogDescription>
							The following tools are a part of the {bundle.name} bundle:
						</DialogDescription>
						{tools.map((tool) => (
							<div key={tool.id} className="flex flex-col">
								<Separator className="mb-4" />
								<div className="flex items-center gap-4">
									<ToolIcon
										className="h-6 w-6"
										name={bundle?.name ?? ""}
										icon={bundle?.metadata?.icon}
									/>
									<div className="flex flex-col gap-2">
										<p className="font-semibold leading-3">{tool.name}</p>
										<p className="leading-5">{tool.description}</p>
									</div>
								</div>
							</div>
						))}
					</div>
				</ScrollArea>
			</DialogContent>
		</Dialog>
	);
}
