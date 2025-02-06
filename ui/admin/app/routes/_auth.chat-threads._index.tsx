import { ReaderIcon } from "@radix-ui/react-icons";
import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { XIcon } from "lucide-react";
import { useMemo, useState } from "react";
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
import { AgentService } from "~/lib/service/api/agentService";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { UserService } from "~/lib/service/api/userService";
import { RouteHandle } from "~/lib/service/routeHandles";
import { RouteQueryParams, RouteService } from "~/lib/service/routeService";
import { timeSince } from "~/lib/utils";

import { DataTable, useRowNavigate } from "~/components/composed/DataTable";
import { SearchInput } from "~/components/composed/SearchInput";
import { Button } from "~/components/ui/button";
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
	const { onRowClick, onCtrlClick } = useRowNavigate<Thread>(
		"/chat-threads/:id",
		"id"
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
			<div className="flex items-center justify-between pb-8">
				<h2>Chat Threads</h2>
				<SearchInput
					onChange={(value) => setSearch(value)}
					placeholder="Search for chat threads..."
				/>
			</div>

			<ThreadFilters userMap={userMap} agentMap={agentMap} />

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
			columnHelper.accessor((thread) => thread.parentName, { header: "Agent" }),
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

function ThreadFilters({
	userMap,
	agentMap,
}: {
	userMap: Map<string, User>;
	agentMap: Map<string, Agent>;
}) {
	const [searchParams] = useSearchParams();
	const navigate = useNavigate();

	const filters = useMemo(() => {
		const query =
			RouteService.getQueryParams("/chat-threads", searchParams.toString()) ??
			{};
		const { from: _, ...filters } = query;

		const updateFilters = (param: keyof typeof filters) => {
			// note(ryanhopperlowe) this is a hack because setting a param to null/undefined
			// appends "null" to the query string.
			const newQuery = structuredClone(query);
			delete newQuery[param];
			return navigate($path("/chat-threads", newQuery));
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
		].filter((x) => !!x);
	}, [agentMap, navigate, searchParams, userMap]);

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
