import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { ShieldAlertIcon } from "lucide-react";
import { useMemo } from "react";
import { MetaFunction } from "react-router";
import { $path } from "safe-routes";
import useSWR, { preload } from "swr";

import { Thread } from "~/lib/model/threads";
import { ExplicitAdminDescription, User, roleLabel } from "~/lib/model/users";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { UserService } from "~/lib/service/api/userService";
import { RouteHandle } from "~/lib/service/routeHandles";
import { pluralize, timeSince } from "~/lib/utils";

import { DataTable } from "~/components/composed/DataTable";
import { Button } from "~/components/ui/button";
import { Link } from "~/components/ui/link";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";
import { UserActionsDropdown } from "~/components/user/UserActionsDropdown";

export async function clientLoader() {
	const users = await preload(UserService.getUsers.key(), UserService.getUsers);

	if (users.length > 0) {
		await preload(ThreadsService.getThreads.key(), ThreadsService.getThreads);
	}

	return null;
}

export default function Users() {
	const { data: users = [] } = useSWR(
		UserService.getUsers.key(),
		UserService.getUsers
	);

	const { data: threads } = useSWR(
		() => users.length > 0 && ThreadsService.getThreads.key(),
		() => ThreadsService.getThreads()
	);

	const userThreadMap = useMemo(() => {
		return threads?.reduce((acc, thread) => {
			if (!thread.userID) return acc;

			if (!acc.has(thread.userID)) acc.set(thread.userID, [thread]);
			else acc.get(thread.userID)?.push(thread);

			return acc;
		}, new Map<string, Thread[]>());
	}, [threads]);

	return (
		<div>
			<div className="flex h-full flex-col gap-4 p-8">
				<h2 className="mb-4">Users</h2>
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
				header: "Email",
			}),
			columnHelper.display({
				id: "thread",
				header: "Thread",
				cell: ({ row }) => {
					const thread = userThreadMap?.get(row.original.id);
					if (thread) {
						return (
							<Link
								to={$path("/threads", {
									userId: row.original.id,
									from: "users",
								})}
								className="underline"
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
						{row.original.explicitAdmin ? (
							<Tooltip>
								<TooltipContent className="max-w-sm">
									{ExplicitAdminDescription}
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
				cell: ({ row }) => (
					<p>{timeSince(new Date(row.original.created))} ago</p>
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
