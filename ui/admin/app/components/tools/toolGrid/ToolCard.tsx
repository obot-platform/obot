import { Trash } from "lucide-react";

import { ToolReference } from "~/lib/model/toolReferences";
import { cn, timeSince } from "~/lib/utils";

import { ConfirmationDialog } from "~/components/composed/ConfirmationDialog";
import { Truncate } from "~/components/composed/typography";
import { ToolIcon } from "~/components/tools/ToolIcon";
import { ToolCardActions } from "~/components/tools/toolGrid/ToolCardActions";
import { Badge } from "~/components/ui/badge";
import { Button } from "~/components/ui/button";
import {
	Card,
	CardContent,
	CardFooter,
	CardHeader,
} from "~/components/ui/card";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";

interface ToolCardProps {
	tool: ToolReference;
	onDelete: (id: string) => void;
}

export function ToolCard({ tool, onDelete }: ToolCardProps) {
	return (
		<Card
			className={cn("flex h-full flex-col", {
				"border-2 border-primary": tool.metadata?.bundle,
				"border-2 border-error": tool.error,
			})}
		>
			<CardHeader className="flex flex-row justify-between space-y-0 pb-2">
				<h4 className="flex flex-wrap items-center gap-x-2">
					<div className="flex flex-nowrap gap-x-2">
						<ToolIcon
							className="h-5 w-5 min-w-5"
							name={tool.name}
							icon={tool.metadata?.icon}
						/>
						<Truncate>{tool.name}</Truncate>
					</div>
					{tool.error && (
						<Tooltip>
							<TooltipTrigger>
								<Badge className="pointer-events-none mb-1 bg-error">
									Failed
								</Badge>
							</TooltipTrigger>
							<TooltipContent className="max-w-xs border border-error bg-error-foreground text-foreground">
								<p>{tool.error}</p>
							</TooltipContent>
						</Tooltip>
					)}
					{tool.metadata?.bundle && (
						<Badge className="pointer-events-none">Bundle</Badge>
					)}
				</h4>

				<ToolCardActions tool={tool} />
			</CardHeader>
			<CardContent className="flex-grow">
				{!tool.builtin && (
					<Truncate className="max-w-full">{tool.reference}</Truncate>
				)}
				<p className="mt-2 line-clamp-2 text-sm text-muted-foreground">
					{tool.description || "No description available"}
				</p>
			</CardContent>
			<CardFooter className="flex h-14 items-center justify-between pt-2">
				<small className="text-muted-foreground">
					{timeSince(new Date(tool.created))} ago
				</small>

				{!tool.builtin && (
					<ConfirmationDialog
						title="Delete Tool Reference"
						description="Are you sure you want to delete this tool reference? This action cannot be undone."
						onConfirm={() => onDelete(tool.id)}
						confirmProps={{
							variant: "destructive",
							children: "Delete",
						}}
					>
						<Button variant="ghost" size="icon">
							<Trash className="h-5 w-5" />
						</Button>
					</ConfirmationDialog>
				)}
			</CardFooter>
		</Card>
	);
}
