import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import {
	CrownIcon,
	EllipsisIcon,
	ExternalLinkIcon,
	GlobeIcon,
	LockIcon,
} from "lucide-react";
import { useMemo } from "react";
import { MetaFunction } from "react-router";
import { $path } from "safe-routes";
import useSWR, { preload } from "swr";

import {
	Project,
	ProjectShare,
	ShareStatus,
	getShareStatusLabel,
} from "~/lib/model/project";
import { getUserDisplayName } from "~/lib/model/users";
import { UserRoutes } from "~/lib/routers/userRoutes";
import { AgentService } from "~/lib/service/api/agentService";
import { ProjectApiService } from "~/lib/service/api/projectApiService";
import { TaskService } from "~/lib/service/api/taskService";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { UserService } from "~/lib/service/api/userService";
import { RouteQueryParams } from "~/lib/service/routeService";
import { pluralize, timeSince } from "~/lib/utils";

import { ConfirmationDialog } from "~/components/composed/ConfirmationDialog";
import {
	DataTable,
	DataTableFilter,
	DataTableTimeFilter,
} from "~/components/composed/DataTable";
import { Filters } from "~/components/composed/Filters";
import { Button } from "~/components/ui/button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "~/components/ui/dropdown-menu";
import { Label } from "~/components/ui/label";
import { Link } from "~/components/ui/link";
import { Switch } from "~/components/ui/switch";
import { useConfirmationDialog } from "~/hooks/component-helpers/useConfirmationDialog";
import { useAsync } from "~/hooks/useAsync";
import { useQueryInfo } from "~/hooks/useRouteInfo";

export type SearchParams = RouteQueryParams<"obotsSchema">;

export async function clientLoader() {
	await Promise.all([
		preload(...ProjectApiService.getAll.swr({})),
		preload(...AgentService.getAgents.swr({})),
		preload(...ThreadsService.getThreads.swr({})),
		preload(...UserService.getUsers.swr({})),
		preload(...ProjectApiService.getAllShares.swr({})),
		preload(...TaskService.getTasks.swr({})),
	]);
}

export default function ProjectsPage() {
	const pageQuery = useQueryInfo("/obots");

	const { data: projects, mutate: refresh } = useSWR(
		...ProjectApiService.getAll.swr({}),
		{ suspense: true }
	);
	const projectMap = useMemo(
		() => new Map(projects.map((p) => [p.id, p])),
		[projects]
	);

	function getChildCount(projectId: string) {
		return projects.filter((p) => p.parentID === projectId).length;
	}

	const { data: shares } = useSWR(...ProjectApiService.getAllShares.swr({}));
	const shareMap = useMemo(() => {
		return new Map(
			shares?.filter((s) => !!s.projectID).map((s) => [s.projectID!, s])
		);
	}, [shares]);

	const filteredProjects = useMemo(() => {
		let filtered = projects;

		const {
			obotId,
			parentObotId,
			showChildren,
			shared,
			createdStart,
			createdEnd,
			agentId,
			userId,
		} = pageQuery.params ?? {};

		if (createdStart) {
			filtered = filtered.filter((p) => {
				const created = new Date(p.created);
				return created >= new Date(createdStart);
			});
		}

		if (createdEnd) {
			filtered = filtered.filter((p) => {
				const created = new Date(p.created);
				return created <= new Date(createdEnd);
			});
		}

		if (shared) {
			filtered = filtered.filter(
				(p) => getShareStatus(shareMap.get(p.id)) === shared
			);
		}

		if (obotId) {
			filtered = filtered.filter((p) => p.id === obotId);
		}

		if (agentId) {
			filtered = filtered.filter((p) => p.assistantID === agentId);
		}

		if (parentObotId) {
			filtered = filtered.filter((p) => p.parentID === parentObotId);
		}

		if (!showChildren) {
			filtered = filtered.filter((p) => !p.parentID);
		}

		if (userId) {
			filtered = filtered.filter((p) => p.userID === userId);
		}

		return filtered;
	}, [projects, pageQuery.params, shareMap]);

	const { data: agents } = useSWR(...AgentService.getAgents.swr({}), {
		suspense: true,
	});
	const agentMap = useMemo(
		() => new Map(agents?.map((a) => [a.id, a])),
		[agents]
	);

	const { data: threads } = useSWR(...ThreadsService.getThreads.swr({}), {
		suspense: true,
	});
	const threadCounts = useMemo(
		() =>
			threads.reduce<Map<string, number>>((acc, thread) => {
				// filter out threads that don't have a parent project, or that are projects themselves
				if (!thread.projectID || thread.project) return acc;

				const count = acc.get(thread.projectID) ?? 0;
				acc.set(thread.projectID, count + 1);

				return acc;
			}, new Map()),
		[threads]
	);

	const { data: tasks } = useSWR(...TaskService.getTasks.swr({}), {
		suspense: true,
	});
	function getTaskCount(projectId: string) {
		return tasks.filter((t) => t.projectID === projectId).length;
	}

	const { data: users } = useSWR(...UserService.getUsers.swr({}), {
		suspense: true,
	});
	const userMap = useMemo(() => new Map(users?.map((u) => [u.id, u])), [users]);

	const { interceptAsync, dialogProps } = useConfirmationDialog();

	const deleteProject = useAsync(ProjectApiService.delete, {
		onSuccess: () => refresh(),
	});

	const handleDelete = (id: string, agentId: string) => {
		interceptAsync(() => deleteProject.executeAsync({ id, agentId }));
	};

	const rows = useMemo(() => {
		const filteredSet = new Set(filteredProjects.map((p) => p.id));

		return filteredProjects.filter(
			// allow top level projects or valid projects who's parent was filtered out
			(p) => !p.parentID || !filteredSet.has(p.parentID)
		);
	}, [filteredProjects]);

	return (
		<div>
			<div className="flex h-full flex-col gap-4 p-8">
				<div className="flex-auto overflow-hidden">
					<div className="width-full mb-8 flex justify-between space-x-2">
						<h2>Obots</h2>
					</div>

					<div className="flex justify-between p-1">
						<Filters
							projectMap={projectMap}
							userMap={userMap}
							agentMap={agentMap}
							url="/obots"
						/>

						<div className="flex items-center gap-2">
							<Label htmlFor="show-children">Include spawned Obots</Label>
							<Switch
								id="show-children"
								checked={!!pageQuery.params?.showChildren}
								onCheckedChange={(checked) => {
									if (checked) pageQuery.update("showChildren", true);
									else pageQuery.remove("showChildren");
								}}
							/>
						</div>
					</div>

					<DataTable
						columns={getColumns()}
						data={rows}
						groupBy={(row) => {
							if (!row.parentID)
								return filteredProjects.filter((p) => p.parentID === row.id);
							else return [];
						}}
						sort={[{ id: "name", desc: false }]}
					/>
				</div>
			</div>

			<ConfirmationDialog
				{...dialogProps}
				title="Delete Obot?"
				description="Are you sure you want to delete this Obot? This action cannot be undone."
				confirmProps={{
					variant: "destructive",
					children: "Delete",
					loading: deleteProject.isLoading,
					disabled: deleteProject.isLoading,
				}}
			/>
		</div>
	);

	function getColumns(): ColumnDef<Project, string>[] {
		return [
			columnHelper.accessor("id", {
				header: "ID",
			}),
			columnHelper.accessor("name", {
				header: "Name",
				cell: ({ row }) => <p>{row.original.name ?? "Untitled"}</p>,
			}),
			columnHelper.accessor(
				(row) => getShareStatus(shareMap.get(row.id)) as string,
				{
					header: ({ column }) => (
						<DataTableFilter
							key={column.id}
							values={Object.values(ShareStatus).map((p) => ({
								id: p,
								name: getShareStatusLabel(p),
							}))}
							field="Privacy"
							onSelect={(value) =>
								pageQuery.update("shared", value as ShareStatus)
							}
						/>
					),
					id: "privacy",
					cell: ({ row }) => {
						const shareState = getShareStatus(shareMap.get(row.original.id));
						return (
							<p className="flex items-center gap-2 [&_svg]:size-4">
								{renderShareIcon(shareState)} {getShareStatusLabel(shareState)}
							</p>
						);
					},
				}
			),
			columnHelper.accessor("parentID", {
				header: ({ column }) => (
					<DataTableFilter
						key={column.id}
						values={projects.map((p) => ({
							id: p.id,
							name: p.name ?? "Untitled",
						}))}
						field="Spawned from"
						onSelect={(value) => pageQuery.update("parentObotId", value)}
					/>
				),
				cell: ({ row }) => {
					if (!row.original.parentID) return "-";

					return (
						<Link to={$path("/obots", { obotId: row.original.parentID })}>
							{projectMap.get(row.original.parentID)?.name}
						</Link>
					);
				},
			}),
			columnHelper.accessor("userID", {
				id: "user",
				header: ({ column }) => (
					<DataTableFilter
						key={column.id}
						values={users?.map((u) => ({ id: u.id, name: u.email }))}
						field="Created By"
						onSelect={(value) => pageQuery.update("userId", value)}
					/>
				),
				cell: ({ getValue }) => {
					if (!getValue()) return "-";

					return (
						<Link to={$path("/users", { userId: getValue() })}>
							{getUserDisplayName(userMap.get(getValue()))}
						</Link>
					);
				},
			}),
			columnHelper.accessor("created", {
				header: ({ column }) => {
					let from: Date | undefined;
					let to: Date | undefined;

					if (pageQuery.params?.createdStart)
						from = new Date(pageQuery.params?.createdStart);

					if (pageQuery.params?.createdEnd)
						to = new Date(pageQuery.params?.createdEnd);

					return (
						<DataTableTimeFilter
							key={column.id}
							field="Created On"
							dateRange={{ from, to }}
							onSelect={(range) => {
								if (!range) pageQuery.remove("createdStart");
								if (range?.from)
									pageQuery.update("createdStart", range.from.toDateString());
								if (range?.to)
									pageQuery.update("createdEnd", range.to.toDateString());
							}}
						/>
					);
				},
				cell: ({ getValue }) => <p>{timeSince(new Date(getValue()))} ago</p>,
			}),
			columnHelper.display({
				id: "info",
				cell: ({ row }) => {
					const childCount = getChildCount(row.original.id);
					const threadCount = threadCounts.get(row.original.id) ?? 0;
					const baseAgent = agentMap.get(row.original.assistantID);
					const taskCount = getTaskCount(row.original.id);

					return (
						<div className="flex flex-col">
							{baseAgent && (
								<p className="flex items-center gap-2 text-muted-foreground">
									<span className="min-w-fit">Base Agent: </span>
									<Link
										to={$path("/agents/:id", { id: row.original.assistantID })}
									>
										{baseAgent.name}
									</Link>
								</p>
							)}

							<p className="flex items-center gap-2">
								{threadCount > 0 && (
									<Link
										to={$path("/chat-threads", {
											obotId: row.original.id,
											from: "obots",
										})}
									>
										{threadCount} {pluralize(threadCount, "thread", "threads")}
									</Link>
								)}
							</p>
							<p className="flex items-center gap-2">
								{childCount > 0 && (
									<Link
										to={$path("/obots", {
											parentObotId: row.original.id,
											showChildren: true,
										})}
									>
										{childCount} spawned Obots
									</Link>
								)}
							</p>

							<p className="flex items-center gap-2">
								{taskCount > 0 && (
									<Link
										to={$path("/tasks", {
											obotId: row.original.id,
										})}
									>
										{taskCount} {pluralize(taskCount, "task")}
									</Link>
								)}
							</p>
						</div>
					);
				},
			}),
			columnHelper.display({
				id: "actions",
				cell: ({ row }) => (
					<DropdownMenu>
						<DropdownMenuTrigger asChild className="float-end">
							<Button variant="ghost" size="icon">
								<EllipsisIcon />
							</Button>
						</DropdownMenuTrigger>

						<DropdownMenuContent side="top" align="end">
							<DropdownMenuItem asChild>
								<a
									href={UserRoutes.obot(row.original.id).url}
									target="_blank"
									rel="noopener noreferrer"
									className="flex items-center gap-2"
								>
									Go to Obot <ExternalLinkIcon className="size-4" />
								</a>
							</DropdownMenuItem>

							<DropdownMenuItem
								variant="destructive"
								onClick={() =>
									handleDelete(row.original.id, row.original.assistantID)
								}
							>
								Delete Obot
							</DropdownMenuItem>
						</DropdownMenuContent>
					</DropdownMenu>
				),
			}),
		];
	}
}

function getShareStatus(share?: ProjectShare) {
	if (share?.featured) return ShareStatus.Featured;
	if (share?.public) return ShareStatus.Public;
	return ShareStatus.Private;
}

function renderShareIcon(privacy: ShareStatus) {
	switch (privacy) {
		case ShareStatus.Featured:
			return <CrownIcon key="star" className="text-warning" />;
		case ShareStatus.Public:
			return <GlobeIcon className="text-primary" key="globe" />;
		case ShareStatus.Private:
			return <LockIcon key="lock" className="text-muted-foreground" />;
	}
}

const columnHelper = createColumnHelper<Project>();

export const meta: MetaFunction = () => {
	return [{ title: "Obot • Obots" }];
};
