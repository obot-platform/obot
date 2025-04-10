import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { useMemo, useState } from "react";
import { ClientLoaderFunctionArgs, MetaFunction } from "react-router";
import { $path } from "safe-routes";
import useSWR, { preload } from "swr";

import { Task } from "~/lib/model/tasks";
import { AgentService } from "~/lib/service/api/agentService";
import { ProjectApiService } from "~/lib/service/api/projectApiService";
import { TaskService } from "~/lib/service/api/taskService";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { UserService } from "~/lib/service/api/userService";
import { RouteHandle } from "~/lib/service/routeHandles";
import { RouteService } from "~/lib/service/routeService";
import { pluralize } from "~/lib/utils";
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
import { useQueryInfo } from "~/hooks/useRouteInfo";

type TableTask = Task & {
	agent: string;
	agentID: string;
	threadCount: number;
	user: string;
	userID: string;
	projectId: string;
	project: string;
};

export async function clientLoader({
	params,
	request,
}: ClientLoaderFunctionArgs) {
	await Promise.all([
		preload(...TaskService.getTasks.swr({})),
		preload(...ThreadsService.getThreads.swr({})),
		preload(...AgentService.getAgents.swr({})),
		preload(...ProjectApiService.getAll.swr({})),
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

	const pageQuery = useQueryInfo("/tasks");

	const getAgents = useSWR(...AgentService.getAgents.swr({}));
	const getUsers = useSWR(...UserService.getUsers.swr({}));
	const getThreads = useSWR(...ThreadsService.getThreads.swr({}));
	const getTasks = useSWR(...TaskService.getTasks.swr({}));
	const getProjects = useSWR(...ProjectApiService.getAll.swr({}));

	const agentMap = useMemo(() => {
		return new Map(getAgents.data?.map((agent) => [agent.id, agent]));
	}, [getAgents.data]);

	const userMap = useMemo(() => {
		return new Map(getUsers.data?.map((user) => [user.id, user]));
	}, [getUsers.data]);

	const taskMap = useMemo(() => {
		return new Map(getTasks.data?.map((task) => [task.id, task]));
	}, [getTasks.data]);

	const projectMap = useMemo(() => {
		return new Map(getProjects.data?.map((project) => [project.id, project]));
	}, [getProjects.data]);

	const tasks: TableTask[] = useMemo(() => {
		const threadCounts = getThreads.data?.reduce<Record<string, number>>(
			(acc, thread) => {
				if (!thread.taskID) return acc;

				acc[thread.taskID] = (acc[thread.taskID] || 0) + 1;
				return acc;
			},
			{}
		);
		return (
			getTasks.data?.map((task) => {
				const project = projectMap.get(task.projectID);
				return {
					...task,
					agentID: project?.assistantID ?? "",
					agent: agentMap.get(project?.assistantID ?? "")?.name ?? "-",
					threadCount: threadCounts?.[task.id] || 0,
					user: userMap.get(project?.userID ?? "")?.email ?? "-",
					userID: project?.userID ?? "",
					projectId: project?.id ?? "",
					project: project?.name ?? "Untitled",
				};
			}) ?? []
		);
	}, [getThreads.data, getTasks.data, projectMap, agentMap, userMap]);

	const data = useMemo(() => {
		let filteredTasks = tasks;

		const { agentId, userId, taskId, createdStart, createdEnd, obotId } =
			pageQuery.params ?? {};

		if (obotId) {
			filteredTasks = filteredTasks.filter((item) => item.projectID === obotId);
		}

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
	}, [tasks, pageQuery.params, search]);

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
						projectMap={projectMap}
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
			columnHelper.accessor("id", {
				header: "ID",
			}),
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
						onSelect={(value) => pageQuery.update("taskId", value)}
					/>
				),
			}),
			columnHelper.accessor(
				(row) => projectMap.get(row.projectID)?.name ?? "Untitled",
				{
					id: "project",
					header: ({ column }) => (
						<DataTableFilter
							key={column.id}
							field="Obot"
							values={
								getProjects.data?.map((project) => ({
									id: project.id,
									name: project.name ?? "Untitled",
								})) ?? []
							}
							onSelect={(value) => pageQuery.update("obotId", value)}
						/>
					),
					cell: (info) => (
						<div className="flex items-center gap-2">
							<Link
								onClick={(e) => e.stopPropagation()}
								to={$path("/obots", {
									obotId: info.row.original.projectId,
								})}
								className="px-0"
							>
								{info.getValue()}
							</Link>
						</div>
					),
				}
			),
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
						onSelect={(value) => pageQuery.update("userId", value)}
					/>
				),
			}),
			columnHelper.accessor((item) => item.threadCount.toString(), {
				id: "tasks",
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
							<p>
								{info.getValue() || 0}{" "}
								{pluralize(Number(info.getValue()), "Run")}
							</p>
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
							from: pageQuery.params?.createdStart
								? new Date(pageQuery.params.createdStart)
								: undefined,
							to: pageQuery.params?.createdEnd
								? new Date(pageQuery.params.createdEnd)
								: undefined,
						}}
						onSelect={(range) => {
							if (range?.from)
								pageQuery.update("createdStart", range.from.toDateString());
							else pageQuery.remove("createdStart");

							if (range?.to)
								pageQuery.update("createdEnd", range.to.toDateString());
							else pageQuery.remove("createdEnd");
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
