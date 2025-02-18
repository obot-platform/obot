import {
	DownloadIcon,
	EditIcon,
	ExternalLink,
	FileIcon,
	FilesIcon,
	KeyIcon,
	LucideIcon,
	PuzzleIcon,
	RotateCwIcon,
	SearchIcon,
	TableIcon,
	TrashIcon,
} from "lucide-react";
import { $path } from "safe-routes";
import useSWR from "swr";

import { Agent } from "~/lib/model/agents";
import { Task } from "~/lib/model/tasks";
import { Thread } from "~/lib/model/threads";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { UserService } from "~/lib/service/api/userService";
import { WorkspaceTableApiService } from "~/lib/service/api/workspaceTableApiService";
import { PaginationInfo } from "~/lib/service/queryService";
import { cn, noop } from "~/lib/utils";

import {
	useThreadCredentials,
	useThreadFiles,
	useThreadKnowledge,
	useThreadTasks,
} from "~/components/chat/shared/thread-helpers";
import { ConfirmationDialog } from "~/components/composed/ConfirmationDialog";
import { PaginationActions } from "~/components/composed/PaginationActions";
import { Truncate } from "~/components/composed/typography";
import { TableNamespace } from "~/components/model/tables";
import { ThreadTableDialog } from "~/components/thread/ThreadTableDialog";
import {
	Accordion,
	AccordionContent,
	AccordionItem,
	AccordionTrigger,
} from "~/components/ui/accordion";
import { Button } from "~/components/ui/button";
import { Card, CardContent } from "~/components/ui/card";
import { ClickableDiv } from "~/components/ui/clickable-div";
import { Input } from "~/components/ui/input";
import { Link } from "~/components/ui/link";
import { Skeleton } from "~/components/ui/skeleton";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";
import { useAuthStatus } from "~/hooks/auth/useAuthStatus";
import { useConfirmationDialog } from "~/hooks/component-helpers/useConfirmationDialog";
import { usePagination } from "~/hooks/pagination/usePagination";

interface ThreadMetaProps {
	entity: Agent | Task;
	thread: Thread;
	className?: string;
}

const pageSize = 10;

export function ThreadMeta({ entity, thread, className }: ThreadMetaProps) {
	const isAgent = entity.type === "agent";
	const from = isAgent
		? $path("/chat-threads/:id", { id: thread.id })
		: $path("/task-runs/:id", { id: entity.id });

	const assistantLink = isAgent
		? $path("/agents/:id", { id: entity.id }, { from })
		: $path("/tasks/:id", { id: entity.id });

	const { authEnabled } = useAuthStatus();

	const fileStore = usePagination({ pageSize });

	const getFiles = useThreadFiles(
		thread.id,
		fileStore.params.pagination,
		fileStore.params.search
	);
	const { items: files } = getFiles.data ?? {};

	if (getFiles.data) fileStore.updateTotal(getFiles.data.total);

	const getKnowledge = useThreadKnowledge(thread.id);
	const { data: knowledge = [] } = getKnowledge;

	const { getCredentials, deleteCredential } = useThreadCredentials(thread.id);
	const { data: credentials = [] } = getCredentials;

	const {
		tasks,
		isLoading: tasksLoading,
		mutate: mutateTasks,
	} = useThreadTasks(isAgent ? thread.id : undefined);

	const { data: user } = useSWR(
		...UserService.getUser.swr({ username: thread.userID })
	);

	const { dialogProps, interceptAsync } = useConfirmationDialog();

	const tableStore = usePagination({ pageSize });
	const getTables = useSWR(
		...WorkspaceTableApiService.getTables.swr({
			namespace: TableNamespace.Threads,
			entityId: thread.id,
			filters: { search: tableStore.search },
			query: { pagination: tableStore.params.pagination },
		})
	);
	const { items: tables } = getTables.data ?? {};

	if (getTables.data) tableStore.updateTotal(getTables.data.total);

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
									{isAgent ? "Agent" : "Task"}
								</td>
								<td className="text-right">
									<div className="flex items-center justify-end gap-2">
										<Link to={assistantLink}>{entity.name}</Link>
									</div>
								</td>
							</tr>
							{thread.userID && authEnabled && (
								<tr className="border-foreground/25">
									<td className="py-2 pr-4 font-medium">User</td>
									<td className="text-right">
										{user?.email || user?.username ? (
											<Link to={$path("/users", { userId: thread.userID })}>
												{user?.email ?? user.username}
											</Link>
										) : (
											"N/A"
										)}
									</td>
								</tr>
							)}
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
							{thread.lastRunID && (
								<tr className="border-foreground/25">
									<td className="py-2 pr-4 font-medium">Last Run ID</td>
									<td className="text-right">{thread.lastRunID}</td>
								</tr>
							)}
						</tbody>
					</table>
				</div>

				<Accordion type="multiple" className="mx-2">
					<ThreadMetaAccordionItem
						value="files"
						icon={FilesIcon}
						title="Files"
						isLoading={getFiles.isValidating}
						onRefresh={() => getFiles.mutate()}
						setSearch={fileStore.debouncedSearch}
						items={files ?? []}
						pagination={fileStore}
						setPage={fileStore.setPage}
						renderItem={(file) => (
							<ClickableDiv
								key={file.name}
								onClick={() =>
									ThreadsService.downloadFile(thread.id, file.name)
								}
							>
								<li className="flex items-center gap-2">
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
						)}
						renderSkeleton={(index) => (
							<div key={index} className="flex items-center gap-2">
								<Skeleton className="rounded-full">
									<Button size="icon-sm" variant="ghost">
										<DownloadIcon />
									</Button>
								</Skeleton>

								<Skeleton className="flex-1 rounded-full">
									<p className="text-transparent">.</p>
								</Skeleton>
							</div>
						)}
					/>

					<ThreadMetaAccordionItem
						value="knowledge"
						icon={FilesIcon}
						title="Knowledge Files"
						isLoading={getKnowledge.isValidating}
						onRefresh={() => getKnowledge.mutate()}
						items={knowledge}
						renderItem={(file) => (
							<li key={file.id} className="flex items-center">
								<FileIcon className="mr-2 h-4 w-4" />
								<p>{file.fileName}</p>
							</li>
						)}
					/>

					<ThreadMetaAccordionItem
						value="credentials"
						icon={KeyIcon}
						title="Credentials"
						isLoading={getCredentials.isValidating}
						onRefresh={() => getCredentials.mutate()}
						items={credentials}
						renderItem={(credential) => (
							<li
								key={credential.name}
								className="flex items-center justify-between"
							>
								<p>{credential.name}</p>

								<Button
									size="icon-sm"
									variant="ghost"
									onClick={() =>
										interceptAsync(() =>
											deleteCredential.executeAsync(credential.name)
										)
									}
								>
									<TrashIcon />
								</Button>
							</li>
						)}
					/>

					<ThreadMetaAccordionItem
						value="tables"
						icon={TableIcon}
						title="Tables"
						isLoading={getTables.isValidating}
						onRefresh={() => getTables.mutate()}
						items={tables ?? []}
						pagination={tableStore}
						setPage={tableStore.setPage}
						renderItem={(table) => (
							<li
								key={table.name}
								className="flex items-center justify-between"
							>
								<p>{table.name}</p>

								<ThreadTableDialog
									threadId={thread.id}
									tableName={table.name}
								/>
							</li>
						)}
						renderSkeleton={(index) => (
							<li key={index} className="flex items-center">
								<Skeleton className="rounded-full">
									<p className="text-transparent">.</p>
								</Skeleton>
							</li>
						)}
					/>

					{isAgent && (
						<ThreadMetaAccordionItem
							value="tasks"
							icon={PuzzleIcon}
							title="Tasks"
							isLoading={tasksLoading}
							onRefresh={() => mutateTasks()}
							items={tasks}
							renderItem={(task) => (
								<li key={task.id} className="flex items-center justify-between">
									<div className="flex items-center">
										<PuzzleIcon className="mr-2 h-4 w-4" />
										<p>{task.name}</p>
										<Link
											to={$path("/tasks/:id", { id: task.id })}
											as="button"
											variant="ghost"
											size="icon-sm"
											target="_blank"
											rel="noreferrer"
										>
											{isAgent ? (
												<EditIcon className="h-4 w-4" />
											) : (
												<ExternalLink className="h-4 w-4" />
											)}
										</Link>
									</div>
									<Link
										to={$path("/task-runs", {
											taskId: task.id,
										})}
									>
										{task.runCount} Runs
									</Link>
								</li>
							)}
						/>
					)}
				</Accordion>

				<ConfirmationDialog
					{...dialogProps}
					title="Delete Credential?"
					description="You will need to re-authenticate to use any tools that require this credential."
					confirmProps={{
						variant: "destructive",
						loading: deleteCredential.isLoading,
						disabled: deleteCredential.isLoading,
					}}
				/>
			</CardContent>
		</Card>
	);
}

type ThreadMetaAccordionItemProps<T> = {
	value: string;
	icon: LucideIcon;
	title: string;
	isLoading?: boolean;
	onRefresh?: (e: React.MouseEvent) => void;
	items: T[];
	renderItem: (item: T) => React.ReactNode;
	renderSkeleton?: (
		index: number,
		renderItem: (item: T) => React.ReactNode
	) => React.ReactNode;
	emptyMessage?: string;
	pagination?: PaginationInfo;
	setPage?: (page: number) => void;
	setSearch?: (search: string) => void;
};

function ThreadMetaAccordionItem<T>(props: ThreadMetaAccordionItemProps<T>) {
	const Icon = props.icon;
	return (
		<AccordionItem value={props.value}>
			<AccordionTrigger>
				<div className="flex w-full items-center justify-between">
					<span className="flex items-center">
						<Icon className="mr-2 h-4 w-4" />
						{props.title}
					</span>

					{props.onRefresh && (
						<Button
							variant="ghost"
							size="icon-sm"
							loading={props.isLoading}
							onClick={(e) => {
								e.stopPropagation();
								props.onRefresh?.(e);
							}}
						>
							<RotateCwIcon />
						</Button>
					)}
				</div>
			</AccordionTrigger>

			<AccordionContent className="mx-4 space-y-2 pt-2">
				{props.setSearch && (
					<Input
						startContent={<SearchIcon />}
						placeholder="Search"
						onChange={(e) => props.setSearch?.(e.target.value)}
					/>
				)}

				<ul className="space-y-2">
					{props.items.length ? (
						props.items.map((item) => props.renderItem(item))
					) : props.isLoading && props.renderSkeleton ? (
						Array.from({ length: props.pagination?.pageSize ?? 10 }).map(
							(_, index) => props.renderSkeleton?.(index, props.renderItem)
						)
					) : (
						<li className="flex items-center">
							<p>{props.emptyMessage || `No ${props.title.toLowerCase()}`}</p>
						</li>
					)}
				</ul>

				{props.pagination && (
					<PaginationActions
						{...props.pagination}
						setPage={props.setPage ?? noop}
					/>
				)}
			</AccordionContent>
		</AccordionItem>
	);
}
