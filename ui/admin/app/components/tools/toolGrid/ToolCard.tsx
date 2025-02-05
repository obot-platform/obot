import { GitCommitIcon } from "lucide-react";

import { ToolReference } from "~/lib/model/toolReferences";
import { cn } from "~/lib/utils/cn";

import { Truncate } from "~/components/composed/typography";
import { ToolIcon } from "~/components/tools/ToolIcon";
import { ToolCardActions } from "~/components/tools/toolGrid/ToolCardActions";
import { Button } from "~/components/ui/button";
import {
	Card,
	CardContent,
	CardFooter,
	CardHeader,
} from "~/components/ui/card";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "~/components/ui/popover";

export function ToolCard({
	tool,
	HeaderRightContent,
}: {
	tool: ToolReference;
	HeaderRightContent?: React.ReactNode;
}) {
	return (
		<Card
			key={tool.id}
			className={cn({
				"border border-destructive bg-destructive/10": tool.error,
			})}
		>
			<CardHeader className="flex min-h-7 flex-row items-center justify-between space-y-0 px-2.5 pb-0 pt-2">
				<div>
					<ToolCardActions tool={tool} />
				</div>
				<div className="pr-2">
					{tool.error ? (
						<Popover>
							<PopoverTrigger asChild>
								<Button size="badge" variant="destructive" className="pr-2">
									Failed
								</Button>
							</PopoverTrigger>
							<PopoverContent className="w-[50vw]">
								<div className="flex flex-col gap-2">
									<p className="text-sm">
										An error occurred during tool registration:
									</p>
									<p className="w-full break-all rounded-md bg-primary-foreground p-2 text-sm text-destructive">
										{tool.error}
									</p>
								</div>
							</PopoverContent>
						</Popover>
					) : (
						HeaderRightContent
					)}
				</div>
			</CardHeader>
			<CardContent className="flex flex-col items-center gap-2 text-center">
				<ToolIcon
					className="h-16 w-16"
					name={tool?.name ?? ""}
					icon={tool?.metadata?.icon}
				/>
				<Truncate className="text-lg font-semibold">{tool.name}</Truncate>
				<Truncate
					classNames={{
						content: "leading-5",
					}}
					className="text-sm"
					clampLength={2}
				>
					{tool.description}
				</Truncate>
				{!tool.builtin && tool.reference && (
					<Truncate
						classNames={{
							content: "leading-5",
						}}
						className="text-wrap break-all text-sm"
						clampLength={2}
					>
						{tool.reference}
					</Truncate>
				)}
			</CardContent>
			{tool.commit && (
				<CardFooter className="flex justify-end">
					<Popover>
						<PopoverTrigger asChild>
							<Button size="icon" variant="ghost">
								<GitCommitIcon className="h-4 w-4" />
							</Button>
						</PopoverTrigger>
						<PopoverContent>
							<div className="flex flex-col gap-2">
								<p className="break-all text-sm text-muted-foreground">
									Commit:{" "}
									{tool.reference?.startsWith("github.com") ? (
										<a
											href={`https://${(() => {
												const parts = tool.reference.split("/");
												if (parts.length >= 3) {
													const [org, repo, ...rest] = parts.slice(1);
													return `github.com/${org}/${repo}/blob/${tool.commit}/${rest.join("/")}/tool.gpt`;
												}
												return tool.reference;
											})()}`}
											target="_blank"
											rel="noopener noreferrer"
											className="text-blue-500 hover:underline"
										>
											{tool.commit}
										</a>
									) : (
										tool.commit
									)}
								</p>
							</div>
						</PopoverContent>
					</Popover>
				</CardFooter>
			)}
		</Card>
	);
}
