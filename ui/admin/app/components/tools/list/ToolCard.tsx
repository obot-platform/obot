import { ToolReference } from "~/lib/model/toolReferences";
import { cn } from "~/lib/utils/cn";

import { Truncate } from "~/components/composed/typography";
import { ToolIcon } from "~/components/tools/ToolIcon";
import { ToolCardActions } from "~/components/tools/list/ToolCardActions";
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
				<ToolCardActions tool={tool} />
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
			<CardContent className="flex flex-col items-center gap-2 text-center">
				<ToolIcon
					className="h-16 w-16"
					disableTooltip
					name={tool?.name ?? ""}
					icon={tool?.metadata?.icon}
				/>
				<Truncate className="text-lg font-semibold">{tool.name}</Truncate>
				<Truncate
					classNames={{
						root: "leading-5",
					}}
					className="text-sm"
					clampLength={2}
				>
					{tool.description}
				</Truncate>
				{!tool.builtin && tool.reference && (
					<Truncate
						classNames={{
							root: "leading-5",
						}}
						className="text-wrap break-all text-sm"
						clampLength={2}
					>
						{tool.reference}
					</Truncate>
				)}
			</CardContent>
		</Card>
	);
}
