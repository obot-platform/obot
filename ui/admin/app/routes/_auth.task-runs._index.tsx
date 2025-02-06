import { ReaderIcon } from "@radix-ui/react-icons";
import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { XIcon } from "lucide-react";
import { useMemo, useState } from "react";
import {
	ClientLoaderFunctionArgs,
	Link,
	MetaFunction,
	useLoaderData,
	useNavigate,
	useSearchParams,
} from "react-router";
import { $path } from "safe-routes";
import useSWR, { preload } from "swr";

import { Thread } from "~/lib/model/threads";
import { User } from "~/lib/model/users";
import { Workflow } from "~/lib/model/workflows";
import { AgentService } from "~/lib/service/api/agentService";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { UserService } from "~/lib/service/api/userService";
import { WorkflowService } from "~/lib/service/api/workflowService";
import { RouteHandle } from "~/lib/service/routeHandles";
import { RouteQueryParams, RouteService } from "~/lib/service/routeService";
import { timeSince } from "~/lib/utils";

import { DataTable, useRowNavigate } from "~/components/composed/DataTable";
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
		preload(UserService.getUsers.key(), UserService.getUsers),
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
	const { onRowClick, onCtrlClick } = useRowNavigate<Thread>(
		"/task-runs/:id",
		"id"
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

	const getUsers = useSWR(UserService.getUsers.key(), UserService.getUsers);

	const threads = useMemo(() => {
		if (!getThreads.data) return [];

		let filteredThreads = getThreads.data.filter(
			(thread) => thread.workflowID && !thread.deleted
		);

		if (taskId) {
			filteredThreads = filteredThreads.filter(
				(thread) => thread.workflowID === taskId
			);
		}

		if (userId) {
			filteredThreads = filteredThreads.filter(
				(thread) => thread.userID === userId
			);
		}

		return filteredThreads;
	}, [getThreads.data, taskId, userId]);

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

	const data: (Thread & { parentName: string; userName: string })[] =
		useMemo(() => {
			return threads.map((thread) => {
				const workflow = workflowMap.get(thread.workflowID!);
				const workflowThread = threadMap.get(workflow?.threadID ?? "");
				return {
					...thread,
					parentName: workflow?.name ?? "Unnamed",
					userName: userMap.get(workflowThread?.userID ?? "")?.email ?? "-",
				};
			});
		}, [threads, userMap, workflowMap, threadMap]);

	const itemsToDisplay = search
		? data.filter(
				(item) =>
					item.parentName.toLowerCase().includes(search.toLowerCase()) ||
					item.userName.toLowerCase().includes(search.toLowerCase())
			)
		: data;

	return (
		<ScrollArea className="flex max-h-full flex-col gap-4 p-8">
			<div className="flex items-center justify-between pb-8">
				<h2>Task Runs</h2>
				<SearchInput
					onChange={(value) => setSearch(value)}
					placeholder="Search for task runs..."
				/>
			</div>

			<ThreadFilters userMap={userMap} workflowMap={workflowMap} />

			<DataTable
				columns={getColumns()}
				data={itemsToDisplay}
				sort={[{ id: "created", desc: true }]}
				disableClickPropagation={(cell) => cell.id.includes("actions")}
				onRowClick={onRowClick}
				onCtrlClick={onCtrlClick}
			/>
		</ScrollArea>
	);

	function getColumns(): ColumnDef<(typeof data)[0], string>[] {
		return [
			columnHelper.accessor((thread) => thread.parentName, { header: "Task" }),
			columnHelper.accessor((thread) => thread.userName, { header: "User" }),
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

function ThreadFilters({
	userMap,
	workflowMap,
}: {
	userMap: Map<string, User>;
	workflowMap: Map<string, Workflow>;
}) {
	const [searchParams] = useSearchParams();
	const navigate = useNavigate();

	const filters = useMemo(() => {
		const query =
			RouteService.getQueryParams("/task-runs", searchParams.toString()) ?? {};
		const { from: _, ...filters } = query;

		const updateFilters = (param: keyof typeof filters) => {
			// note(ryanhopperlowe) this is a hack because setting a param to null/undefined
			// appends "null" to the query string.
			const newQuery = structuredClone(query);
			delete newQuery[param];
			return navigate($path("/task-runs", newQuery));
		};

		return [
			filters.userId && {
				key: "userId",
				label: "User",
				value: userMap.get(filters.userId)?.email ?? filters.userId,
				onRemove: () => updateFilters("userId"),
			},
			filters.taskId && {
				key: "taskId",
				label: "Task",
				value: workflowMap.get(filters.taskId)?.name ?? filters.taskId,
				onRemove: () => updateFilters("taskId"),
			},
		].filter((x) => !!x);
	}, [navigate, searchParams, userMap, workflowMap]);

	return (
		<div className="flex gap-2">
			{filters.map((filter) => (
				<Button
					key={filter.key}
					size="badge"
					onClick={filter.onRemove}
					variant="accent"
					shape="pill"
					endContent={<XIcon />}
				>
					<b>{filter.label}:</b> {filter.value}
				</Button>
			))}
		</div>
	);
}

const columnHelper = createColumnHelper<
	Thread & { parentName: string; userName: string }
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
