import {
	DownloadIcon,
	EditIcon,
	ExternalLink,
	FileIcon,
	FilesIcon,
} from "lucide-react";
import { $path } from "safe-routes";

import { Agent } from "~/lib/model/agents";
import { KnowledgeFile } from "~/lib/model/knowledge";
import { runStateToBadgeColor } from "~/lib/model/runs";
import { Thread } from "~/lib/model/threads";
import { Workflow } from "~/lib/model/workflows";
import { WorkspaceFile } from "~/lib/model/workspace";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { cn } from "~/lib/utils";

import { Truncate } from "~/components/composed/typography";
import {
	Accordion,
	AccordionContent,
	AccordionItem,
	AccordionTrigger,
} from "~/components/ui/accordion";
import { Badge } from "~/components/ui/badge";
import { Button } from "~/components/ui/button";
import { Card, CardContent } from "~/components/ui/card";
import { ClickableDiv } from "~/components/ui/clickable-div";
import { Link } from "~/components/ui/link";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";

interface ThreadMetaProps {
	for: Agent | Workflow;
	thread: Thread;
	files: WorkspaceFile[];
	knowledge: KnowledgeFile[];
	className?: string;
}

export function ThreadMeta({
	for: entity,
	thread,
	files,
	className,
	knowledge,
}: ThreadMetaProps) {
	const from = $path("/threads/:id", { id: thread.id });
	const isAgent = entity.id.startsWith("a");

	const assistantLink = isAgent
		? $path("/agents/:agent", { agent: entity.id }, { from })
		: $path("/workflows/:workflow", { workflow: entity.id });

	return (
		<Card className={cn("bg-0 h-full overflow-hidden", className)}>
			<CardContent className="space-y-4 pt-6">
				<div className="overflow-hidden rounded-md bg-muted p-4">
					<table className="w-full">
						<tbody>
							<tr className="border-foreground/25">
								<td className="py-2 pr-4 font-medium">Created</td>
								<td className="text-right">
									{new Date(thread.created).toLocaleString()}
								</td>
							</tr>
							<tr className="border-foreground/25">
								<td className="py-2 pr-4 font-medium">
									{isAgent ? "Agent" : "Workflow"}
								</td>
								<td className="text-right">
									<div className="flex items-center justify-end gap-2">
										<span>{entity.name}</span>

										<Link
											to={assistantLink}
											as="button"
											variant="ghost"
											size="icon"
										>
											{isAgent ? (
												<EditIcon className="h-4 w-4" />
											) : (
												<ExternalLink className="h-4 w-4" />
											)}
										</Link>
									</div>
								</td>
							</tr>
							<tr className="border-foreground/25">
								<td className="py-2 pr-4 font-medium">State</td>
								<td className="text-right">
									<Badge
										variant="outline"
										className={cn(
											runStateToBadgeColor(thread.state),
											"text-white"
										)}
									>
										{thread.state}
									</Badge>
								</td>
							</tr>
							{thread.currentRunId && (
								<tr className="border-foreground/25">
									<td className="py-2 pr-4 font-medium">Current Run ID</td>
									<td className="text-right">{thread.currentRunId}</td>
								</tr>
							)}
							{thread.parentThreadId && (
								<tr className="border-foreground/25">
									<td className="py-2 pr-4 font-medium">Parent Thread ID</td>
									<td className="text-right">{thread.parentThreadId}</td>
								</tr>
							)}
							{thread.lastRunId && (
								<tr className="border-foreground/25">
									<td className="py-2 pr-4 font-medium">Last Run ID</td>
									<td className="text-right">{thread.lastRunId}</td>
								</tr>
							)}
						</tbody>
					</table>
				</div>

				<Accordion type="multiple" className="mx-2">
					{files.length > 0 && (
						<AccordionItem value="files">
							<AccordionTrigger>
								<span className="flex items-center">
									<FilesIcon className="mr-2 h-4 w-4" />
									Files
								</span>
							</AccordionTrigger>
							<AccordionContent className="mx-4">
								<ul className="space-y-2">
									{files.map((file) => (
										<ClickableDiv
											key={file.name}
											onClick={() =>
												ThreadsService.downloadFile(thread.id, file.name)
											}
										>
											<li key={file.name} className="flex items-center gap-2">
												<Tooltip>
													<TooltipTrigger asChild>
														<Button variant="ghost" size="icon-sm">
															<DownloadIcon />
														</Button>
													</TooltipTrigger>

													<TooltipContent>Download</TooltipContent>
												</Tooltip>
												<Truncate
													className="w-fit flex-1"
													tooltipContentProps={{ align: "start" }}
												>
													{file.name}
												</Truncate>
											</li>
										</ClickableDiv>
									))}
								</ul>
							</AccordionContent>
						</AccordionItem>
					)}
					{knowledge.length > 0 && (
						<AccordionItem value="knowledge">
							<AccordionTrigger>
								<span className="flex items-center text-base">
									<FilesIcon className="mr-2 h-4 w-4" />
									Knowledge Files
								</span>
							</AccordionTrigger>
							<AccordionContent className="mx-4">
								<ul className="space-y-2">
									{knowledge.map((file) => (
										<li key={file.id} className="flex items-center">
											<FileIcon className="mr-2 h-4 w-4" />
											<span>{file.fileName}</span>
										</li>
									))}
								</ul>
							</AccordionContent>
						</AccordionItem>
					)}
				</Accordion>
			</CardContent>
		</Card>
	);
}
