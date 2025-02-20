import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { useMemo, useState } from "react";
import {
	ClientLoaderFunctionArgs,
	MetaFunction,
	useLoaderData,
} from "react-router";
import { $path } from "safe-routes";
import useSWR, { preload } from "swr";

import { Task } from "~/lib/model/tasks";
import { AgentService } from "~/lib/service/api/agentService";
import { TaskService } from "~/lib/service/api/taskService";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { UserService } from "~/lib/service/api/userService";
import { RouteHandle } from "~/lib/service/routeHandles";
import { RouteService } from "~/lib/service/routeService";
import { filterByCreatedRange } from "~/lib/utils/filter";
import { timeSince } from "~/lib/utils/time";

import {
	DataTable,
	DataTableFilter,
	DataTableTimeFilter,
	useRowNavigate,
} from "~/components/composed/DataTable";
import { Filters } from "~/components/composed/Filters";
import { SearchInput } from "~/components/composed/SearchInput";
import { Link } from "~/components/ui/link";

type TableTask = Task & {
	agent: string;
	agentID: string;
	threadCount: number;
	user: string;
	userID: string;
};

export async function clientLoader({
	params,
	request,
}: ClientLoaderFunctionArgs) {
	await Promise.all([
		preload(...TaskService.getTasks.swr({})),
		preload(...ThreadsService.getThreads.swr({})),
		preload(...AgentService.getAgents.swr({})),
	]);

	const { query } = RouteService.getRouteInfo(
		"/task-runs",
		new URL(request.url),
		params
	);

	return query ?? {};
}

export default function Tasks() {
	const [search, setSearch] = useState("");
	const navigate = useRowNavigate((value: Task | string) =>
		typeof value === "string" ? value : $path("/tasks/:id", { id: value.id })
	);
	const { taskId, userId, agentId, createdStart, createdEnd } =
		useLoaderData<typeof clientLoader>();

	const getAgents = useSWR(...AgentService.getAgents.swr({}));
	const getUsers = useSWR(...UserService.getUsers.swr({}));

	const getThreads = useSWR(...ThreadsService.getThreads.swr({}));
	const getTasks = useSWR(...TaskService.getTasks.swr({}));

	const agentMap = useMemo(() => {
		return new Map(getAgents.data?.map((agent) => [agent.id, agent]));
	}, [getAgents.data]);

	const userMap = useMemo(() => {
		return new Map(getUsers.data?.map((user) => [user.id, user]));
	}, [getUsers.data]);

	const taskMap = useMemo(() => {
		return new Map(getTasks.data?.map((task) => [task.id, task]));
	}, [getTasks.data]);

	const tasks: TableTask[] = useMemo(() => {
		const threadsMap = new Map(
			getThreads.data?.map((thread) => [thread.id, thread])
		);

		const threadCounts = getThreads.data?.reduce<Record<string, number>>(
			(acc, thread) => {
				if (!thread.workflowID) return acc;

				acc[thread.workflowID] = (acc[thread.workflowID] || 0) + 1;
				return acc;
			},
			{}
		);
		return (
			getTasks.data?.map((task) => {
				const rootThread = threadsMap.get(task.threadID ?? "");
				return {
					...task,
					agentID: rootThread?.agentID ?? "",
					agent: agentMap.get(rootThread?.agentID ?? "")?.name ?? "-",
					threadCount: threadCounts?.[task.id] || 0,
					user: userMap.get(rootThread?.userID ?? "")?.email ?? "-",
					userID: rootThread?.userID ?? "",
				};
			}) ?? []
		);
		return [];
	}, [getTasks.data, agentMap, getThreads.data, userMap]);

	const data = useMemo(() => {
		let filteredTasks = tasks;

		if (agentId) {
			filteredTasks = filteredTasks.filter((item) => item.agentID === agentId);
		}

		if (userId) {
			filteredTasks = filteredTasks.filter((item) => item.userID === userId);
		}

		if (taskId) {
			filteredTasks = filteredTasks.filter((item) => item.id === taskId);
		}

		if (createdStart) {
			filteredTasks = filterByCreatedRange(
				filteredTasks,
				createdStart,
				createdEnd
			);
		}

		filteredTasks = search
			? filteredTasks.filter(
					(item) =>
						item.name.toLowerCase().includes(search.toLowerCase()) ||
						item.agent.toLowerCase().includes(search.toLowerCase()) ||
						item.user.toLowerCase().includes(search.toLowerCase())
				)
			: filteredTasks;

		return filteredTasks;
	}, [tasks, search, agentId, userId, taskId, createdStart, createdEnd]);

	const namesCount = useMemo(() => {
		return data.reduce<Record<string, number>>((acc, task) => {
			acc[task.name] = (acc[task.name] || 0) + 1;
			return acc;
		}, {});
	}, [data]);

	return (
		<div>
			<div className="flex h-full flex-col gap-4 p-6">
				<div className="flex-auto overflow-hidden">
					<div className="flex items-center justify-between pb-8">
						<h2>Tasks</h2>
						<SearchInput
							onChange={(value) => setSearch(value)}
							placeholder="Search for tasks..."
						/>
					</div>

					<Filters
						userMap={userMap}
						agentMap={agentMap}
						taskMap={taskMap}
						url="/tasks"
					/>

					<DataTable
						columns={getColumns()}
						data={data}
						sort={[{ id: "created", desc: true }]}
						onRowClick={navigate.internal}
						onCtrlClick={navigate.external}
					/>
				</div>
			</div>
		</div>
	);

	function getColumns(): ColumnDef<(typeof tasks)[0], string>[] {
		return [
			columnHelper.accessor("name", {
				id: "Task",
				header: ({ column }) => (
					<DataTableFilter
						key={column.id}
						field="Task"
						values={
							data?.map((task) => ({
								id: task.id,
								name: task.name,
								sublabel: namesCount[task.name] > 1 ? task.agent : "",
							})) ?? []
						}
						onSelect={(value) => {
							navigate.internal(
								$path("/tasks", {
									taskId: value,
									...(agentId && { agentId }),
									...(userId && { userId }),
								})
							);
						}}
					/>
				),
			}),
			columnHelper.accessor("agent", {
				id: "Agent",
				header: ({ column }) => (
					<DataTableFilter
						key={column.id}
						field="Agent"
						values={
							getAgents.data?.map((agent) => ({
								id: agent.id,
								name: agent.name,
							})) ?? []
						}
						onSelect={(value) => {
							navigate.internal(
								$path("/tasks", {
									agentId: value,
									...(taskId && { taskId }),
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
							to={$path("/agents/:id", {
								id: info.row.original.agentID,
							})}
							className="px-0"
						>
							<p>{info.getValue()}</p>
						</Link>
					</div>
				),
			}),
			columnHelper.accessor("user", {
				id: "User",
				header: ({ column }) => (
					<DataTableFilter
						key={column.id}
						field="User"
						values={
							getUsers.data?.map((user) => ({
								id: user.id,
								name: user.email,
							})) ?? []
						}
						onSelect={(value) => {
							navigate.internal(
								$path("/tasks", {
									userId: value,
									...(taskId && { taskId }),
									...(agentId && { agentId }),
								})
							);
						}}
					/>
				),
			}),
			columnHelper.accessor((item) => item.threadCount.toString(), {
				id: "tasks-action",
				header: "Runs",
				cell: (info) => (
					<div className="flex items-center gap-2">
						<Link
							onClick={(event) => event.stopPropagation()}
							to={$path("/task-runs", {
								taskId: info.row.original.id,
								from: "tasks",
							})}
							className="px-0"
						>
							<p>{info.getValue() || 0} Runs</p>
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
								$path("/tasks", {
									createdStart: range?.from?.toDateString() ?? "",
									createdEnd: range?.to?.toDateString() ?? "",
									...(taskId && { taskId }),
									...(agentId && { agentId }),
									...(userId && { userId }),
								})
							);
						}}
					/>
				),
				cell: (info) => (
					<p>{timeSince(new Date(info.row.original.created))} ago</p>
				),
			}),
		];
	}
}

const columnHelper = createColumnHelper<TableTask>();

export const handle: RouteHandle = {
	breadcrumb: () => [{ content: "Tasks" }],
};

export const meta: MetaFunction = () => {
	return [{ title: `Obot • Tasks` }];
};
