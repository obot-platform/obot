import { ReactNode } from "react";

import { ToolCall } from "~/lib/model/chatEvents";

import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "~/components/ui/popover";

interface ToolCallInfoProps {
	tool: ToolCall;
	children: ReactNode;
}

export function ToolCallInfo({ tool, children }: ToolCallInfoProps) {
	return (
		<Popover>
			<PopoverTrigger asChild>{children}</PopoverTrigger>
			<PopoverContent className="w-80" side="left">
				<div className="space-y-4">
					<div className="space-y-2">
						<h4 className="font-medium leading-none">{tool.name}</h4>
						<p className="text-sm text-muted-foreground">{tool.description}</p>
						<h3 className="text-sm font-medium">Input</h3>
						<p className="text-wrap break-words rounded-md bg-gray-100 p-2 text-sm text-muted-foreground">
							{tool.input}
						</p>
					</div>
				</div>
			</PopoverContent>
		</Popover>
	);
}
