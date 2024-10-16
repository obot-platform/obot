import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { useMemo } from "react";
import useSWR from "swr";

import { OAuthApp, OAuthAppSpec } from "~/lib/model/oauthApps";
import { OauthAppService } from "~/lib/service/api/oauthAppService";
import { cn } from "~/lib/utils";

import { DataTable } from "~/components/composed/DataTable";

import { DeleteOAuthApp } from "./DeleteOAuthApp";
import { EditOAuthApp } from "./EditOAuthApp";

type Row = OAuthApp & { created?: string; isGateway?: boolean };
const columnHelper = createColumnHelper<Row>();

export function OAuthAppList({
    defaultData,
    spec,
}: {
    defaultData: OAuthApp[];
    spec: OAuthAppSpec;
}) {
    const { data: apps } = useSWR(
        OauthAppService.getOauthApps.key(),
        OauthAppService.getOauthApps,
        { fallbackData: defaultData }
    );

    const rows = useMemo<Row[]>(() => {
        const typesWithNoApps = Object.entries(spec)
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
            columnHelper.accessor((app) => app.name ?? app.id, {
                id: "name",
                header: "Name / Id",
            }),
            columnHelper.accessor((app) => spec[app.type].displayName, {
                id: "type",
                header: "Type",
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
                            <EditOAuthApp
                                oauthApp={row.original}
                                appSpec={spec}
                            />
                            <DeleteOAuthApp id={row.original.id} />
                        </div>
                    ),
            }),
        ];
    }
}
