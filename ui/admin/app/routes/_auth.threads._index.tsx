import { PersonIcon, ReaderIcon } from "@radix-ui/react-icons";
import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { PuzzleIcon, Trash, XIcon } from "lucide-react";
import { useMemo } from "react";
import {
	ClientLoaderFunctionArgs,
	MetaFunction,
	useLoaderData,
	useNavigate,
	useSearchParams,
} from "react-router";
import { $path } from "safe-routes";
import useSWR, { preload } from "swr";

import { Agent } from "~/lib/model/agents";
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

import { DataTable } from "~/components/composed/DataTable";
import { Button } from "~/components/ui/button";
import { Link } from "~/components/ui/link";
import { ScrollArea } from "~/components/ui/scroll-area";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";
import { useAsync } from "~/hooks/useAsync";

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
		"/threads",
		new URL(request.url),
		params
	);

	return query ?? {};
}

export default function Threads() {
	const navigate = useNavigate();
	const { agentId, workflowId, userId } = useLoaderData<typeof clientLoader>();

	const getThreads = useSWR(
		ThreadsService.getThreads.key(),
		ThreadsService.getThreads
	);

	const getAgents = useSWR(...AgentService.getAgents.swr({}));

	const getWorkflows = useSWR(
		WorkflowService.getWorkflows.key(),
		WorkflowService.getWorkflows
	);

	const getUsers = useSWR(UserService.getUsers.key(), UserService.getUsers);

	const threads = useMemo(() => {
		if (!getThreads.data) return [];

		let filteredThreads = getThreads.data.filter(
			(thread) => (thread.agentID || thread.workflowID) && !thread.deleted
		);

		if (agentId) {
			filteredThreads = filteredThreads.filter(
				(thread) => thread.agentID === agentId
			);
		}

		if (workflowId) {
			filteredThreads = filteredThreads.filter(
				(thread) => thread.workflowID === workflowId
			);
		}

		if (userId) {
			filteredThreads = filteredThreads.filter(
				(thread) => thread.userID === userId
			);
		}

		return filteredThreads;
	}, [getThreads.data, agentId, workflowId, userId]);

	const agentMap = useMemo(
		() => new Map(getAgents.data?.map((agent) => [agent.id, agent])),
		[getAgents.data]
	);
	const workflowMap = useMemo(
		() =>
			new Map(getWorkflows.data?.map((workflow) => [workflow.id, workflow])),
		[getWorkflows.data]
	);
	const userMap = useMemo(
		() => new Map(getUsers.data?.map((user) => [user.id, user])),
		[getUsers.data]
	);

	const data: (Thread & { parentName: string; userName: string })[] =
		useMemo(() => {
			return threads.map((thread) => ({
				...thread,
				parentName:
					(thread.agentID
						? agentMap.get(thread.agentID)?.name
						: thread.workflowID
							? workflowMap.get(thread.workflowID)?.name
							: "Unnamed") ?? "Unnamed",
				userName: thread.userID
					? (userMap.get(thread.userID)?.email ?? "-")
					: "-",
			}));
		}, [threads, agentMap, userMap, workflowMap]);

	const deleteThread = useAsync(ThreadsService.deleteThread, {
		onSuccess: ThreadsService.revalidateThreads,
	});

	return (
		<ScrollArea className="flex max-h-full flex-col gap-4 p-8">
			<h2>Threads</h2>

			<ThreadFilters
				userMap={userMap}
				agentMap={agentMap}
				workflowMap={workflowMap}
			/>

			<DataTable
				columns={getColumns()}
				data={data}
				sort={[{ id: "created", desc: true }]}
				disableClickPropagation={(cell) => cell.id.includes("actions")}
				onRowClick={(row) => {
					navigate($path("/threads/:id", { id: row.id }));
				}}
			/>
		</ScrollArea>
	);

	function getColumns(): ColumnDef<(typeof data)[0], string>[] {
		return [
			columnHelper.accessor((thread) => thread.parentName, { header: "Name" }),
			columnHelper.display({
				id: "type",
				header: "Type",
				cell: ({ row }) => {
					return (
						<p className="flex items-center gap-2">
							{row.original.agentID ? (
								<PersonIcon className="h-4 w-4" />
							) : (
								<PuzzleIcon className="h-4 w-4" />
							)}
							{row.original.agentID ? "Agent" : "Workflow"}
						</p>
					);
				},
			}),
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
								<Link
									to={$path("/threads/:id", {
										id: row.original.id,
									})}
									as="button"
									variant="ghost"
									size="icon"
								>
									<ReaderIcon width={21} height={21} />
								</Link>
							</TooltipTrigger>

							<TooltipContent>
								<p>Inspect Thread</p>
							</TooltipContent>
						</Tooltip>

						<Tooltip>
							<TooltipTrigger asChild>
								<Button
									variant="ghost"
									size="icon"
									onClick={() => deleteThread.execute(row.original.id)}
								>
									<Trash />
								</Button>
							</TooltipTrigger>

							<TooltipContent>
								<p>Delete Thread</p>
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
	agentMap,
	workflowMap,
}: {
	userMap: Map<string, User>;
	agentMap: Map<string, Agent>;
	workflowMap: Map<string, Workflow>;
}) {
	const [searchParams] = useSearchParams();
	const navigate = useNavigate();

	const filters = useMemo(() => {
		const query =
			RouteService.getQueryParams("/threads", searchParams.toString()) ?? {};
		const { from: _, ...filters } = query;

		const updateFilters = (param: keyof typeof filters) => {
			// note(ryanhopperlowe) this is a hack because setting a param to null/undefined
			// appends "null" to the query string.
			const newQuery = structuredClone(query);
			delete newQuery[param];
			return navigate($path("/threads", newQuery));
		};

		return [
			filters.agentId && {
				key: "agentId",
				label: "Agent",
				value: agentMap.get(filters.agentId)?.name ?? filters.agentId,
				onRemove: () => updateFilters("agentId"),
			},
			filters.userId && {
				key: "userId",
				label: "User",
				value: userMap.get(filters.userId)?.email ?? filters.userId,
				onRemove: () => updateFilters("userId"),
			},
			filters.workflowId && {
				key: "workflowId",
				label: "Workflow",
				value: workflowMap.get(filters.workflowId)?.name ?? filters.workflowId,
				onRemove: () => updateFilters("workflowId"),
			},
		].filter((x) => !!x);
	}, [agentMap, navigate, searchParams, userMap, workflowMap]);

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
	const { from } = RouteService.getQueryParams("/threads", search) || {};

	if (from === "agents")
		return {
			content: "Agents",
			href: $path("/agents"),
		};

	if (from === "users")
		return {
			content: "Users",
			href: $path("/users"),
		};

	if (from === "workflows")
		return {
			content: "Workflows",
			href: $path("/workflows"),
		};
};

export const handle: RouteHandle = {
	breadcrumb: ({ search }) =>
		[getFromBreadcrumb(search), { content: "Threads" }].filter((x) => !!x),
};

export const meta: MetaFunction = () => {
	return [{ title: `Obot • Threads` }];
};
