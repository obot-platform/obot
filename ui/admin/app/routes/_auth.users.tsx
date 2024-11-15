import { Link } from "@remix-run/react";
import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { useMemo } from "react";
import { $path } from "remix-routes";
import useSWR, { preload } from "swr";

import { Thread } from "~/lib/model/threads";
import { User, roleToString } from "~/lib/model/users";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { UserService } from "~/lib/service/api/userService";
import { pluralize, timeSince } from "~/lib/utils";

import { TypographyH2, TypographyP } from "~/components/Typography";
import { DataTable } from "~/components/composed/DataTable";

export async function clientLoader() {
    const users = await preload(
        UserService.getUsers.key(),
        UserService.getUsers
    );

    if (users.length > 0) {
        await preload(
            ThreadsService.getThreads.key(),
            ThreadsService.getThreads
        );
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
            <div className="h-full p-8 flex flex-col gap-4">
                <TypographyH2 className="mb-4">Users</TypographyH2>
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
                    return <TypographyP>No Threads</TypographyP>;
                },
            }),
            columnHelper.display({
                id: "role",
                header: "Role",
                cell: ({ row }) => (
                    <TypographyP>{roleToString(row.original.role)}</TypographyP>
                ),
            }),
            columnHelper.display({
                id: "created",
                header: "Created",
                cell: ({ row }) => (
                    <TypographyP>
                        {timeSince(new Date(row.original.created))} ago
                    </TypographyP>
                ),
            }),
        ];
    }
}

const columnHelper = createColumnHelper<User>();
