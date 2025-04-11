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
import { ProjectApiService } from "~/lib/service/api/projectApiService";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { UserService } from "~/lib/service/api/userService";
import { RouteHandle } from "~/lib/service/routeHandles";
import { RouteQueryParams, RouteService } from "~/lib/service/routeService";
import { timeSince } from "~/lib/utils";
import { filterByCreatedRange } from "~/lib/utils/filter";

import {
	DataTable,
	DataTableFilter,
	DataTableTimeFilter,
	useRowNavigate,
} from "~/components/composed/DataTable";
import { Filters } from "~/components/composed/Filters";
import { SearchInput } from "~/components/composed/SearchInput";
import { DeleteThread } from "~/components/thread/DeleteThread";
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
		preload(...ThreadsService.getThreads.swr({})),
		preload(...UserService.getUsers.swr({})),
		preload(...ProjectApiService.getAll.swr({})),
	]);

	const { query } = RouteService.getRouteInfo(
		"/chat-threads",
		new URL(request.url),
		params
	);

	return query ?? {};
}

export default function ChatThreads() {
	const [search, setSearch] = useState("");
	const navigate = useRowNavigate((value: Thread | string) =>
		typeof value === "string"
			? value
			: $path("/chat-threads/:id", { id: value.id })
	);
	const {
		agentId,
		userId,
		createdStart,
		createdEnd,
		obotId: obotId,
	} = useLoaderData<typeof clientLoader>();

	const getThreads = useSWR(...ThreadsService.getThreads.swr({}));
	const getAgents = useSWR(...AgentService.getAgents.swr({}));
	const getUsers = useSWR(...UserService.getUsers.swr({}));
	const getProjects = useSWR(...ProjectApiService.getAll.swr({}));

	const threads = useMemo(() => {
		if (!getThreads.data) return [];

		let filteredThreads = getThreads.data.filter(
			(thread) =>
				thread.assistantID &&
				!thread.deleted &&
				!thread.project &&
				!thread.taskID
		);

		if (agentId) {
			filteredThreads = filteredThreads.filter(
				(thread) => thread.assistantID === agentId
			);
		}

		if (obotId) {
			filteredThreads = filteredThreads.filter(
				(thread) => thread.projectID === obotId
			);
		}

		if (userId) {
			filteredThreads = filteredThreads.filter(
				(thread) => thread.userID === userId
			);
		}

		if (createdStart) {
			filteredThreads = filterByCreatedRange(
				filteredThreads,
				createdStart,
				createdEnd
			);
		}

		return filteredThreads;
	}, [getThreads.data, agentId, obotId, userId, createdStart, createdEnd]);

	const agentMap = useMemo(
		() => new Map(getAgents.data?.map((agent) => [agent.id, agent])),
		[getAgents.data]
	);
	const userMap = useMemo(
		() => new Map(getUsers.data?.map((user) => [user.id, user])),
		[getUsers.data]
	);
	const projectMap = useMemo(
		() => new Map(getProjects.data?.map((project) => [project.id, project])),
		[getProjects.data]
	);

	const data: (Thread & { parentName: string; userName: string })[] =
		useMemo(() => {
			return threads.map((thread) => ({
				...thread,
				parentName:
					(thread.projectID && projectMap.get(thread.projectID)?.name) ??
					"Untitled",
				userName: thread.userID
					? (userMap.get(thread.userID)?.email ?? "-")
					: "-",
			}));
		}, [threads, projectMap, userMap]);

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

			<Filters
				userMap={userMap}
				agentMap={agentMap}
				projectMap={projectMap}
				url="/chat-threads"
			/>

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
				id: "Obot",
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
						onSelect={(value) => {
							navigate.internal(
								$path("/chat-threads", {
									obotId: value,
									...(userId && { userId }),
									...(createdStart && { createdStart }),
									...(createdEnd && { createdEnd }),
								})
							);
						}}
					/>
				),
				cell: (info) => (
					<div className="flex items-center gap-2">
						<Link
							onClick={(event) => event.stopPropagation()}
							to={$path("/obots", {
								obotId: info.row.original.projectID!,
								showChildren: true,
							})}
							className="px-0"
						>
							<p>{info.getValue()}</p>
						</Link>
					</div>
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
									...(createdStart && { createdStart }),
									...(createdEnd && { createdEnd }),
								})
							);
						}}
					/>
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
								$path("/chat-threads", {
									createdStart: range?.from?.toDateString() ?? "",
									createdEnd: range?.to?.toDateString() ?? "",
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

						<DeleteThread id={row.original.id} />
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

	if (from === "obots") {
		return {
			content: "Obots",
			href: $path("/obots"),
		};
	}

	return null;
};

export const handle: RouteHandle = {
	breadcrumb: ({ search }) =>
		[getFromBreadcrumb(search), { content: "Chat Threads" }].filter((x) => !!x),
};

export const meta: MetaFunction = () => {
	return [{ title: `Obot • Chat Threads` }];
};
