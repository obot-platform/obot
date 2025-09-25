import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { ShieldAlertIcon } from "lucide-react";
import { useMemo } from "react";
import {
	ClientLoaderFunctionArgs,
	MetaFunction,
	useLoaderData,
	useNavigate,
} from "react-router";
import { $path } from "safe-routes";
import useSWR, { preload } from "swr";

import { Thread } from "~/lib/model/threads";
import { ExplicitRoleDescription, User, roleLabel } from "~/lib/model/users";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { UserService } from "~/lib/service/api/userService";
import { VersionApiService } from "~/lib/service/api/versionApiService";
import { RouteHandle } from "~/lib/service/routeHandles";
import { RouteQueryParams, RouteService } from "~/lib/service/routeService";
import { daysSince, pluralize } from "~/lib/utils";

import { DataTable, DataTableFilter } from "~/components/composed/DataTable";
import { Filters } from "~/components/composed/Filters";
import { Button } from "~/components/ui/button";
import { Link } from "~/components/ui/link";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";
import { UserActionsDropdown } from "~/components/user/UserActionsDropdown";

export type SearchParams = RouteQueryParams<"usersSchema">;

export async function clientLoader({ request }: ClientLoaderFunctionArgs) {
	await VersionApiService.requireAuthEnabled();

	const query = RouteService.getQueryParams(
		"/users",
		new URL(request.url).search
	);

	const users = await preload(...UserService.getUsers.swr({}));

	if (users.length > 0) {
		await preload(...ThreadsService.getThreads.swr({}));
	}

	return { filters: query };
}

export default function Users() {
	const { filters } = useLoaderData<typeof clientLoader>();
	const getUsers = useSWR(...UserService.getUsers.swr({}));

	const users = useMemo(() => {
		if (!getUsers.data) return [];

		return filters?.userId
			? getUsers.data.filter((user) => user.id === filters.userId)
			: getUsers.data;
	}, [getUsers.data, filters]);

	const { data: threads } = useSWR(
		...ThreadsService.getThreads.swr({}, { enabled: !!users.length })
	);

	const userThreadMap = useMemo(() => {
		return threads?.reduce((acc, thread) => {
			if (!thread.userID) return acc;

			if (!acc.has(thread.userID)) acc.set(thread.userID, [thread]);
			else acc.get(thread.userID)?.push(thread);

			return acc;
		}, new Map<string, Thread[]>());
	}, [threads]);

	const userMap = new Map(users.map((u) => [u.id, u]));

	const navigate = useNavigate();

	return (
		<div>
			<div className="flex h-full flex-col gap-4 p-8">
				<h2 className="mb-4">Users</h2>

				<Filters userMap={userMap} url="/users" />

				<DataTable
					columns={getColumns()}
					data={users}
					sort={[{ id: "created", desc: true }]}
				/>
			</div>
		</div>
	);

	function getColumns(): ColumnDef<User, string>[] {
		return [
			columnHelper.accessor("email", {
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
						onSelect={(userId) => navigate($path("/users", { userId }))}
					/>
				),
			}),
			columnHelper.display({
				id: "thread",
				header: "Thread",
				cell: ({ row }) => {
					const thread = userThreadMap?.get(row.original.id);
					if (thread) {
						return (
							<Link
								to={$path("/chat-threads", {
									userId: row.original.id,
									from: "users",
								})}
							>
								View {thread.length}{" "}
								{pluralize(thread.length, "Thread", "Threads")}
							</Link>
						);
					}
					return <p>No Threads</p>;
				},
			}),
			columnHelper.accessor((row) => roleLabel(row.role), {
				id: "role",
				header: "Role",
				cell: ({ row, getValue }) => (
					<div>
						{row.original.explicitRole ? (
							<Tooltip>
								<TooltipContent className="max-w-sm">
									{ExplicitRoleDescription}
								</TooltipContent>

								<div className="flex items-center gap-2">
									{getValue()}

									<TooltipTrigger asChild>
										<Button size="icon" variant="ghost">
											<ShieldAlertIcon />
										</Button>
									</TooltipTrigger>
								</div>
							</Tooltip>
						) : (
							getValue()
						)}
					</div>
				),
			}),
			columnHelper.display({
				id: "created",
				header: "Created",
				cell: ({ row }) => <p>{daysSince(new Date(row.original.created))}</p>,
			}),
			columnHelper.display({
				id: "lastActiveDay",
				header: "Last Active",
				cell: ({ row }) =>
					row.original.lastActiveDay ? (
						<p>{daysSince(new Date(row.original.lastActiveDay))}</p>
					) : (
						<p>No activity</p>
					),
			}),
			columnHelper.display({
				id: "actions",
				cell: ({ row }) => (
					<div className="flex justify-end">
						<UserActionsDropdown user={row.original} />
					</div>
				),
			}),
		];
	}
}

const columnHelper = createColumnHelper<User>();

export const handle: RouteHandle = {
	breadcrumb: () => [{ content: "Users" }],
};

export const meta: MetaFunction = () => {
	return [{ title: `Obot • Users` }];
};
