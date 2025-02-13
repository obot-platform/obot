import { CheckIcon, ChevronUpIcon } from "lucide-react";
import React, { useState } from "react";
import useSWR from "swr";

import { Thread } from "~/lib/model/threads";
import { ThreadsService } from "~/lib/service/api/threadsService";

import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Button } from "~/components/ui/button";
import {
	Command,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
} from "~/components/ui/command";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "~/components/ui/popover";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";

interface PastThreadsProps {
	agentId: string;
	currentThreadId?: string | null;
	onThreadSelect: (threadId: string) => void;
}

export const PastThreads: React.FC<PastThreadsProps> = ({
	agentId,
	currentThreadId,
	onThreadSelect,
}) => {
	const [open, setOpen] = useState(false);
	const {
		data: threads,
		error,
		isLoading,
		mutate,
	} = useSWR(...ThreadsService.getThreadsByAgent.swr({ agentId }));

	const handleOpenChange = (newOpen: boolean) => {
		setOpen(newOpen);
		if (newOpen) {
			mutate();
		}
	};

	const handleThreadSelect = (threadId: string) => {
		onThreadSelect(threadId);
		setOpen(false);
	};

	return (
		<Tooltip>
			<TooltipContent>Switch threads</TooltipContent>

			<Popover open={open} onOpenChange={handleOpenChange}>
				<PopoverTrigger asChild>
					<TooltipTrigger asChild>
						<Button variant="ghost" size="icon">
							<ChevronUpIcon className="h-4 w-4" />
						</Button>
					</TooltipTrigger>
				</PopoverTrigger>

				<PopoverContent className="w-80 p-0">
					<Command className="flex-col-reverse">
						<CommandInput placeholder="Search threads..." />
						<CommandList>
							<CommandEmpty>No threads found.</CommandEmpty>
							{isLoading ? (
								<div className="flex h-20 items-center justify-center">
									<LoadingSpinner size={24} />
								</div>
							) : error ? (
								<div className="p-2 text-center text-red-500">
									Failed to load threads
								</div>
							) : threads && threads.length > 0 ? (
								<CommandGroup>
									{threads.map((thread: Thread) => (
										<CommandItem
											key={thread.id}
											onSelect={() => handleThreadSelect(thread.id)}
											className="cursor-pointer"
										>
											<div className="flex w-full items-center justify-between">
												<div>
													<p className="font-semibold">
														Thread
														<span className="ml-2 text-muted-foreground">
															{thread.id}
														</span>
													</p>
													<p className="text-sm text-gray-500">
														{new Date(thread.created).toLocaleString()}
													</p>
												</div>
												<div>
													{currentThreadId && thread.id === currentThreadId && (
														<CheckIcon />
													)}
												</div>
											</div>
										</CommandItem>
									))}
								</CommandGroup>
							) : null}
						</CommandList>
					</Command>
				</PopoverContent>
			</Popover>
		</Tooltip>
	);
};
