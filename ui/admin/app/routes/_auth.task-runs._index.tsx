import { ReaderIcon } from "@radix-ui/react-icons";
import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { useMemo, useState } from "react";
import {
	ClientLoaderFunctionArgs,
	Link,
	MetaFunction,
	useLoaderData,
} from "react-router";
import { $path } from "safe-routes";
import useSWR, { preload } from "swr";

import { Thread } from "~/lib/model/threads";
import { AgentService } from "~/lib/service/api/agentService";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { UserService } from "~/lib/service/api/userService";
import { WorkflowService } from "~/lib/service/api/workflowService";
import { RouteHandle } from "~/lib/service/routeHandles";
import { RouteQueryParams, RouteService } from "~/lib/service/routeService";
import { timeSince } from "~/lib/utils";

import {
	DataTable,
	DataTableFilter,
	useRowNavigate,
} from "~/components/composed/DataTable";
import { Filters } from "~/components/composed/Filters";
import { SearchInput } from "~/components/composed/SearchInput";
import { Button } from "~/components/ui/button";
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
		preload(WorkflowService.getWorkflows.key(), WorkflowService.getWorkflows),
		preload(ThreadsService.getThreads.key(), ThreadsService.getThreads),
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
	const { taskId, userId } = useLoaderData<typeof clientLoader>();

	const getThreads = useSWR(
		ThreadsService.getThreads.key(),
		ThreadsService.getThreads
	);

	const getWorkflows = useSWR(
		WorkflowService.getWorkflows.key(),
		WorkflowService.getWorkflows
	);

	const getAgents = useSWR(...AgentService.getAgents.swr({}));
	const getUsers = useSWR(...UserService.getUsers.swr({}));

	const agentThreadMap = useMemo(() => {
		const agentMap = new Map(getAgents.data?.map((agent) => [agent.id, agent]));
		return new Map(
			getThreads?.data
				?.filter((thread) => thread.agentID)
				.map((thread) => {
					const agent = agentMap.get(thread.agentID!);
					return [thread.id, agent?.name ?? "-"];
				})
		);
	}, [getAgents.data, getThreads.data]);

	const workflowMap = useMemo(
		() =>
			new Map(getWorkflows.data?.map((workflow) => [workflow.id, workflow])),
		[getWorkflows.data]
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
				?.filter((thread) => thread.workflowID && !thread.deleted)
				.map((thread) => {
					const workflow = workflowMap.get(thread.workflowID!);
					const workflowThread = threadMap.get(workflow?.threadID ?? "");
					return {
						...thread,
						parentName: workflow?.name ?? "Unnamed",
						userName: userMap.get(workflowThread?.userID ?? "")?.email ?? "-",
						userID: workflowThread?.userID ?? "",
					};
				}) ?? []
		);
	}, [getThreads.data, userMap, workflowMap, threadMap]);

	const data = useMemo(() => {
		let filteredThreads = threads;

		if (taskId) {
			filteredThreads = threads.filter(
				(thread) => thread.workflowID === taskId
			);
		}

		if (userId) {
			filteredThreads = threads.filter((thread) => thread.userID === userId);
		}

		return filteredThreads;
	}, [threads, taskId, userId]);

	const namesCount = useMemo(() => {
		return (
			getWorkflows.data?.reduce<Record<string, number>>((acc, workflow) => {
				acc[workflow.name] = (acc[workflow.name] || 0) + 1;
				return acc;
			}, {}) ?? {}
		);
	}, [getWorkflows.data]);

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

			<Filters userMap={userMap} workflowMap={workflowMap} url="/task-runs" />

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
			columnHelper.accessor((thread) => thread.parentName, {
				id: "Task",
				header: ({ column }) => (
					<DataTableFilter
						key={column.id}
						field="Task"
						values={
							getWorkflows.data?.map((workflow) => ({
								id: workflow.id,
								name: workflow.name,
								sublabel:
									namesCount?.[workflow.name] > 1
										? agentThreadMap.get(workflow.threadID ?? "")
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
			}),
			columnHelper.accessor((thread) => thread.userName, {
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
								$path("/task-runs", {
									userId: value,
									...(taskId && { taskId }),
								})
							);
						}}
					/>
				),
			}),
			columnHelper.accessor("created", {
				id: "created",
				header: "Created",
				cell: (info) => (
					<p>{timeSince(new Date(info.row.original.created))} ago</p>
				),
				sortingFn: "datetime",
			}),
			columnHelper.display({
				id: "actions",
				cell: ({ row }) => (
					<div className="flex justify-end gap-2">
						<Tooltip>
							<TooltipTrigger asChild>
								<Button variant="ghost" size="icon">
									<Link
										to={$path("/task-runs/:id", {
											id: row.original.id,
										})}
									>
										<ReaderIcon width={21} height={21} />
									</Link>
								</Button>
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
