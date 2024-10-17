import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { KeyIcon } from "lucide-react";
import { useMemo } from "react";
import useSWR from "swr";

import { OAuthApp } from "~/lib/model/oauthApps";
import { OauthAppService } from "~/lib/service/api/oauthAppService";
import { cn } from "~/lib/utils";

import { DataTable } from "~/components/composed/DataTable";
import { useOAuthAppSpec } from "~/hooks/oauthApps/useOAuthAppSpec";

import { DeleteOAuthApp } from "./DeleteOAuthApp";
import { EditOAuthApp } from "./EditOAuthApp";

type Row = OAuthApp & { created?: string; isGateway?: boolean };
const columnHelper = createColumnHelper<Row>();

export function OAuthAppList() {
    const { data: spec } = useOAuthAppSpec();

    const { data: apps } = useSWR(
        OauthAppService.getOauthApps.key(),
        OauthAppService.getOauthApps,
        { fallbackData: [] }
    );

    const rows = useMemo<Row[]>(() => {
        const typesWithNoApps = Array.from(spec.entries())
            .map(([type, { displayName }]) => {
                if (apps.some((app) => app.type === type)) return null;

                return {
                    type,
                    name: displayName + " (Acorn Gateway)",
                    id: type,
                    isGateway: true,
                } as Row;
            })
            .filter((x) => !!x);

        return apps.concat(typesWithNoApps);
    }, [apps, spec]);

    return (
        <DataTable
            data={rows}
            columns={getColumns()}
            sort={[
                { id: "type", desc: true },
                { id: "name", desc: true },
            ]}
            rowClassName={(row) => cn(row.isGateway && "opacity-60")}
            classNames={{
                row: "!max-h-[200px] grow-0  height-[200px]",
                cell: "!max-h-[200px] grow-0 height-[200px]",
            }}
        />
    );

    function getColumns(): ColumnDef<Row, string>[] {
        return [
            columnHelper.display({
                id: "icon",
                cell: ({ row }) => {
                    const app = row.original;
                    const { icon } = spec.get(app.type) || {};
                    return icon ? (
                        <img
                            src={icon}
                            alt={app.type}
                            className={cn("w-4 h-4", {
                                invisible: !icon,
                            })}
                        />
                    ) : (
                        <KeyIcon className="w-4 h-4" />
                    );
                },
            }),
            columnHelper.accessor(
                (app) => spec.get(app.type)?.displayName ?? app.type,
                {
                    id: "type",
                    header: "Type",
                }
            ),
            columnHelper.accessor((app) => app.name ?? app.id, {
                id: "name",
                header: "Name / Id",
            }),
            columnHelper.accessor(
                (app) =>
                    app.created ? new Date(app.created).toLocaleString() : "-",
                { header: "Created" }
            ),
            columnHelper.display({
                id: "actions",
                cell: ({ row }) =>
                    !row.original.isGateway && (
                        <div className="flex justify-end gap-2">
                            <EditOAuthApp oauthApp={row.original} />
                            <DeleteOAuthApp id={row.original.id} />
                        </div>
                    ),
            }),
        ];
    }
}
