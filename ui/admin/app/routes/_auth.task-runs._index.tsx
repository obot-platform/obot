import { ReaderIcon } from "@radix-ui/react-icons";
import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { useMemo, useState } from "react";
import {
	ClientLoaderFunctionArgs,
	MetaFunction,
	useLoaderData,
} from "react-router";
import { $path } from "safe-routes";
import useSWR, { preload } from "swr";

import { Thread } from "~/lib/model/threads";
import { AgentService } from "~/lib/service/api/agentService";
import { TaskService } from "~/lib/service/api/taskService";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { UserService } from "~/lib/service/api/userService";
import { RouteHandle } from "~/lib/service/routeHandles";
import { RouteQueryParams, RouteService } from "~/lib/service/routeService";
import { formatTime } from "~/lib/utils";
import { filterByCreatedRange } from "~/lib/utils/filter";

import {
	DataTable,
	DataTableFilter,
	DataTableTimeFilter,
	useRowNavigate,
} from "~/components/composed/DataTable";
import { Filters } from "~/components/composed/Filters";
import { SearchInput } from "~/components/composed/SearchInput";
import { Link } from "~/components/ui/link";
import { ScrollArea } from "~/components/ui/scroll-area";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";

export type SearchParams = RouteQueryParams<"threadsListSchema">;

export async function clientLoader({
	params,
	request,
}: ClientLoaderFunctionArgs) {
	await Promise.all([
		preload(...AgentService.getAgents.swr({})),
		preload(...TaskService.getTasks.swr({})),
		preload(...ThreadsService.getThreads.swr({})),
		preload(...UserService.getUsers.swr({})),
	]);

	const { query } = RouteService.getRouteInfo(
		"/task-runs",
		new URL(request.url),
		params
	);

	return query ?? {};
}

export default function TaskRuns() {
	const [search, setSearch] = useState("");
	const navigate = useRowNavigate((value: Thread | string) =>
		typeof value === "string"
			? value
			: $path("/task-runs/:id", { id: value.id })
	);
	const { taskId, userId, createdStart, createdEnd } =
		useLoaderData<typeof clientLoader>();

	const getThreads = useSWR(...ThreadsService.getThreads.swr({}));
	const getTasks = useSWR(...TaskService.getTasks.swr({}));
	const getAgents = useSWR(...AgentService.getAgents.swr({}));
	const getUsers = useSWR(...UserService.getUsers.swr({}));

	const agentThreadMap = useMemo(() => {
		const agentMap = new Map(getAgents.data?.map((agent) => [agent.id, agent]));
		return new Map(
			getThreads?.data
				?.filter((thread) => thread.assistantID)
				.map((thread) => {
					const agent = agentMap.get(thread.assistantID!);
					return [thread.id, agent?.name ?? "-"];
				})
		);
	}, [getAgents.data, getThreads.data]);

	const taskMap = useMemo(
		() => new Map(getTasks.data?.map((task) => [task.id, task])),
		[getTasks.data]
	);
	const userMap = useMemo(
		() => new Map(getUsers.data?.map((user) => [user.id, user])),
		[getUsers.data]
	);
	const threadMap = useMemo(
		() => new Map(getThreads.data?.map((thread) => [thread.id, thread])),
		[getThreads.data]
	);

	const threads: (Thread & {
		parentName: string;
		userName: string;
		userID: string;
	})[] = useMemo(() => {
		return (
			getThreads.data
				?.filter((thread) => thread.taskID && !thread.deleted)
				.map((thread) => {
					const task = taskMap.get(thread.taskID!);
					const taskThread = threadMap.get(task?.projectID ?? "");
					return {
						...thread,
						parentName: task?.name ?? "Untitled",
						userName: userMap.get(taskThread?.userID ?? "")?.email ?? "-",
						userID: taskThread?.userID ?? "",
					};
				}) ?? []
		);
	}, [getThreads.data, userMap, taskMap, threadMap]);

	const data = useMemo(() => {
		let filteredThreads = threads;

		if (taskId) {
			filteredThreads = threads.filter((thread) => thread.taskID === taskId);
		}

		if (userId) {
			filteredThreads = threads.filter((thread) => thread.userID === userId);
		}

		if (createdStart) {
			filteredThreads = filterByCreatedRange(
				filteredThreads,
				createdStart,
				createdEnd
			);
		}

		return filteredThreads;
	}, [threads, taskId, userId, createdStart, createdEnd]);

	const namesCount = useMemo(() => {
		return (
			getTasks.data?.reduce<Record<string, number>>((acc, task) => {
				acc[task.name] = (acc[task.name] || 0) + 1;
				return acc;
			}, {}) ?? {}
		);
	}, [getTasks.data]);

	const itemsToDisplay = search
		? data.filter(
				(item) =>
					item.parentName.toLowerCase().includes(search.toLowerCase()) ||
					item.userName.toLowerCase().includes(search.toLowerCase())
			)
		: data;

	return (
		<ScrollArea className="flex max-h-full flex-col gap-4 p-8">
			<div className="flex items-center justify-between pb-6">
				<h2>Task Runs</h2>
				<SearchInput
					onChange={(value) => setSearch(value)}
					placeholder="Search for task runs..."
				/>
			</div>

			<Filters userMap={userMap} taskMap={taskMap} url="/task-runs" />

			<DataTable
				columns={getColumns()}
				data={itemsToDisplay}
				sort={[{ id: "created", desc: true }]}
				disableClickPropagation={(cell) => cell.id.includes("actions")}
				onRowClick={navigate.internal}
				onCtrlClick={navigate.external}
			/>
		</ScrollArea>
	);

	function getColumns(): ColumnDef<(typeof data)[0], string>[] {
		return [
			columnHelper.accessor("id", {
				header: "ID",
			}),
			columnHelper.accessor((thread) => thread.parentName, {
				id: "Task",
				header: ({ column }) => (
					<DataTableFilter
						key={column.id}
						field="Task"
						values={
							getTasks.data?.map((task) => ({
								id: task.id,
								name: task.name,
								sublabel:
									namesCount?.[task.name] > 1
										? agentThreadMap.get(task.projectID ?? "")
										: "",
							})) ?? []
						}
						onSelect={(value) => {
							navigate.internal(
								$path("/task-runs", {
									taskId: value,
									...(userId && { userId }),
								})
							);
						}}
					/>
				),
				cell: (info) => (
					<div className="flex items-center gap-2">
						<Link
							onClick={(event) => event.stopPropagation()}
							to={$path("/tasks/:id", {
								id: info.row.original.taskID!,
							})}
							className="px-0"
						>
							<p>{info.getValue()}</p>
						</Link>
					</div>
				),
			}),
			columnHelper.accessor("created", {
				id: "created",
				header: ({ column }) => (
					<DataTableTimeFilter
						key={column.id}
						field="Created"
						dateRange={{
							from: createdStart ? new Date(createdStart) : undefined,
							to: createdEnd ? new Date(createdEnd) : undefined,
						}}
						onSelect={(range) => {
							navigate.internal(
								$path("/task-runs", {
									createdStart: range?.from?.toDateString() ?? "",
									createdEnd: range?.to?.toDateString() ?? "",
									...(taskId && { taskId }),
									...(userId && { userId }),
								})
							);
						}}
					/>
				),
				cell: (info) => (
					<p>{formatTime(new Date(info.row.original.created))}</p>
				),
				sortingFn: "datetime",
			}),
			columnHelper.display({
				id: "actions",
				cell: ({ row }) => (
					<div className="flex justify-end gap-2">
						<Tooltip>
							<TooltipTrigger asChild>
								<Link
									as="button"
									to={$path("/task-runs/:id", {
										id: row.original.id,
									})}
									size="icon"
									variant="ghost"
								>
									<ReaderIcon width={21} height={21} />
								</Link>
							</TooltipTrigger>

							<TooltipContent>
								<p>Inspect Thread</p>
							</TooltipContent>
						</Tooltip>
					</div>
				),
			}),
		];
	}
}

const columnHelper = createColumnHelper<
	Thread & { parentName: string; userName: string; userID: string }
>();

const getFromBreadcrumb = (search: string) => {
	const { from } = RouteService.getQueryParams("/task-runs", search) || {};
	if (from === "users")
		return {
			content: "Users",
			href: $path("/users"),
		};

	if (from === "tasks")
		return {
			content: "Tasks",
			href: $path("/tasks"),
		};
};

export const handle: RouteHandle = {
	breadcrumb: ({ search }) =>
		[getFromBreadcrumb(search), { content: "Task Runs" }].filter((x) => !!x),
};

export const meta: MetaFunction = () => {
	return [{ title: `Obot • Task Runs` }];
};
