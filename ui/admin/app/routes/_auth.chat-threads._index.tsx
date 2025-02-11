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
import { ThreadsService } from "~/lib/service/api/threadsService";
import { UserService } from "~/lib/service/api/userService";
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
		preload(ThreadsService.getThreads.key(), ThreadsService.getThreads),
		preload(UserService.getUsers.key(), UserService.getUsers),
	]);

	const { query } = RouteService.getRouteInfo(
		"/chat-threads",
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
			: $path("/chat-threads/:id", { id: value.id })
	);
	const { agentId, userId } = useLoaderData<typeof clientLoader>();

	const getThreads = useSWR(
		ThreadsService.getThreads.key(),
		ThreadsService.getThreads
	);

	const getAgents = useSWR(...AgentService.getAgents.swr({}));
	const getUsers = useSWR(UserService.getUsers.key(), UserService.getUsers);

	const threads = useMemo(() => {
		if (!getThreads.data) return [];

		let filteredThreads = getThreads.data.filter(
			(thread) => thread.agentID && !thread.deleted
		);

		if (agentId) {
			filteredThreads = filteredThreads.filter(
				(thread) => thread.agentID === agentId
			);
		}

		if (userId) {
			filteredThreads = filteredThreads.filter(
				(thread) => thread.userID === userId
			);
		}

		return filteredThreads;
	}, [getThreads.data, agentId, userId]);

	const agentMap = useMemo(
		() => new Map(getAgents.data?.map((agent) => [agent.id, agent])),
		[getAgents.data]
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
					(thread.agentID && agentMap.get(thread.agentID)?.name) ?? "Unnamed",
				userName: thread.userID
					? (userMap.get(thread.userID)?.email ?? "-")
					: "-",
			}));
		}, [threads, agentMap, userMap]);

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
				<h2>Chat Threads</h2>
				<SearchInput
					onChange={(value) => setSearch(value)}
					placeholder="Search for chat threads..."
				/>
			</div>

			<Filters userMap={userMap} agentMap={agentMap} url="/chat-threads" />

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
								$path("/chat-threads", {
									agentId: value,
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
								$path("/chat-threads", {
									userId: value,
									...(agentId && { agentId }),
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
								<Link
									to={$path("/chat-threads/:id", {
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
					</div>
				),
			}),
		];
	}
}

const columnHelper = createColumnHelper<
	Thread & { parentName: string; userName: string }
>();

const getFromBreadcrumb = (search: string) => {
	const { from } = RouteService.getQueryParams("/chat-threads", search) || {};

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
};

export const handle: RouteHandle = {
	breadcrumb: ({ search }) =>
		[getFromBreadcrumb(search), { content: "Chat Threads" }].filter((x) => !!x),
};

export const meta: MetaFunction = () => {
	return [{ title: `Obot • Chat Threads` }];
};
