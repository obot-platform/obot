import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { useMemo, useState } from "react";
import {
	ClientLoaderFunctionArgs,
	MetaFunction,
	useLoaderData,
} from "react-router";
import { $path } from "safe-routes";
import useSWR, { preload } from "swr";

import { Workflow } from "~/lib/model/workflows";
import { AgentService } from "~/lib/service/api/agentService";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { UserService } from "~/lib/service/api/userService";
import { WorkflowService } from "~/lib/service/api/workflowService";
import { RouteHandle } from "~/lib/service/routeHandles";
import { RouteService } from "~/lib/service/routeService";
import { timeSince } from "~/lib/utils/time";

import {
	DataTable,
	DataTableFilter,
	useRowNavigate,
} from "~/components/composed/DataTable";
import { Filters } from "~/components/composed/Filters";
import { SearchInput } from "~/components/composed/SearchInput";
import { Link } from "~/components/ui/link";

type Task = Workflow & {
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
		preload(WorkflowService.getWorkflows.key(), WorkflowService.getWorkflows),
		preload(ThreadsService.getThreads.key(), ThreadsService.getThreads),
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
	const navigate = useRowNavigate((value: Workflow | string) =>
		typeof value === "string" ? value : $path("/tasks/:id", { id: value.id })
	);
	const { taskId, userId, agentId } = useLoaderData<typeof clientLoader>();

	const getAgents = useSWR(...AgentService.getAgents.swr({}));
	const getUsers = useSWR(UserService.getUsers.key(), UserService.getUsers);

	const getThreads = useSWR(
		ThreadsService.getThreads.key(),
		ThreadsService.getThreads
	);
	const getWorkflows = useSWR(
		WorkflowService.getWorkflows.key(),
		WorkflowService.getWorkflows
	);

	const agentMap = useMemo(() => {
		return new Map(getAgents.data?.map((agent) => [agent.id, agent]));
	}, [getAgents.data]);

	const userMap = useMemo(() => {
		return new Map(getUsers.data?.map((user) => [user.id, user]));
	}, [getUsers.data]);

	const workflowMap = useMemo(() => {
		return new Map(
			getWorkflows.data?.map((workflow) => [workflow.id, workflow])
		);
	}, [getWorkflows.data]);

	const tasks: Task[] = useMemo(() => {
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
			getWorkflows.data?.map((workflow) => {
				const rootThread = threadsMap.get(workflow.threadID ?? "");
				return {
					...workflow,
					agentID: rootThread?.agentID ?? "",
					agent: agentMap.get(rootThread?.agentID ?? "")?.name ?? "-",
					threadCount: threadCounts?.[workflow.id] || 0,
					user: userMap.get(rootThread?.userID ?? "")?.email ?? "-",
					userID: rootThread?.userID ?? "",
				};
			}) ?? []
		);
	}, [getWorkflows.data, agentMap, getThreads.data, userMap]);

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

		filteredTasks = search
			? filteredTasks.filter(
					(item) =>
						item.name.toLowerCase().includes(search.toLowerCase()) ||
						item.agent.toLowerCase().includes(search.toLowerCase()) ||
						item.user.toLowerCase().includes(search.toLowerCase())
				)
			: filteredTasks;

		return filteredTasks;
	}, [tasks, search, agentId, userId, taskId]);

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
						workflowMap={workflowMap}
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
				header: "Created",
				cell: (info) => (
					<p>{timeSince(new Date(info.row.original.created))} ago</p>
				),
			}),
		];
	}
}

const columnHelper = createColumnHelper<Task>();

export const handle: RouteHandle = {
	breadcrumb: () => [{ content: "Tasks" }],
};

export const meta: MetaFunction = () => {
	return [{ title: `Obot • Tasks` }];
};
