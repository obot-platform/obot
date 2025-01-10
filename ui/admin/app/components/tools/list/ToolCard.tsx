import { ToolReference } from "~/lib/model/toolReferences";
import { cn } from "~/lib/utils/cn";

import { ToolIcon } from "~/components/tools/ToolIcon";
import { ToolDropdown } from "~/components/tools/list/ToolDropdown";
import { Button } from "~/components/ui/button";
import { Card, CardContent, CardHeader } from "~/components/ui/card";
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
			<CardHeader className="flex flex-row items-center justify-between space-y-0 px-2.5 pb-0 pt-2">
				{!tool.builtin || tool?.metadata?.oauth ? (
					<ToolDropdown tool={tool} />
				) : (
					<div className="h-8 w-6" />
				)}
				<div className="pr-2">
					{tool.error ? (
						<Popover>
							<PopoverTrigger asChild>
								<Button size="badge" variant="destructive">
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
			<CardContent className="flex flex-col items-center gap-4">
				<ToolIcon
					className="h-16 w-16"
					disableTooltip
					name={tool?.name ?? ""}
					icon={tool?.metadata?.icon}
				/>
				<div className="line-clamp-1 text-center text-lg font-semibold">
					{tool.name}
				</div>
				<p className="line-clamp-2 text-center text-sm">{tool.description}</p>
				{!tool.builtin && tool.reference && (
					<p className="line-clamp-2 text-wrap break-all text-center text-sm">
						{tool.reference}
					</p>
				)}
			</CardContent>
		</Card>
	);
}
