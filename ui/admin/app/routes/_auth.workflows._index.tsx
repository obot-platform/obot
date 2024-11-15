import { Link, useNavigate } from "@remix-run/react";
import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { PenSquareIcon } from "lucide-react";
import { useMemo } from "react";
import { $path } from "remix-routes";
import useSWR, { mutate, preload } from "swr";

import { Workflow } from "~/lib/model/workflows";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { WorkflowService } from "~/lib/service/api/workflowService";
import { timeSince } from "~/lib/utils";

import { TypographyH2, TypographyP } from "~/components/Typography";
import { DataTable } from "~/components/composed/DataTable";
import { Button } from "~/components/ui/button";
import {
    Tooltip,
    TooltipContent,
    TooltipTrigger,
} from "~/components/ui/tooltip";
import { CreateWorkflow } from "~/components/workflow/CreateWorkflow";
import { DeleteWorkflowButton } from "~/components/workflow/DeleteWorkflow";
import { WorkflowViewYaml } from "~/components/workflow/WorkflowView";

export async function clientLoader() {
    mutate(WorkflowService.getWorkflows.key(), ThreadsService.getThreads.key());
    await Promise.all([
        preload(
            WorkflowService.getWorkflows.key(),
            WorkflowService.getWorkflows
        ),
        preload(ThreadsService.getThreads.key(), ThreadsService.getThreads),
    ]);
    return null;
}

export default function Workflows() {
    const navigate = useNavigate();
    const getWorkflows = useSWR(
        WorkflowService.getWorkflows.key(),
        WorkflowService.getWorkflows
    );

    const getThreads = useSWR(
        ThreadsService.getThreads.key(),
        ThreadsService.getThreads
    );

    const threadCounts = useMemo(() => {
        if (
            !getWorkflows.data ||
            !getThreads.data ||
            !Array.isArray(getWorkflows.data)
        )
            return {};

        return getThreads.data?.reduce(
            (acc, thread) => {
                if (!thread.workflowID) return acc;

                acc[thread.workflowID] = (acc[thread.workflowID] || 0) + 1;
                return acc;
            },
            {} as Record<string, number>
        );
    }, [getThreads.data, getWorkflows.data]);

    return (
        <div>
            <div className="h-full p-8 flex flex-col gap-4">
                <div className="flex-auto overflow-hidden">
                    <div className="flex space-x-2 width-full justify-between mb-8">
                        <TypographyH2>Workflows</TypographyH2>

                        <CreateWorkflow />
                    </div>

                    <DataTable
                        columns={getColumns()}
                        data={getWorkflows.data || []}
                        sort={[{ id: "created", desc: true }]}
                        disableClickPropagation={(cell) =>
                            cell.id.includes("action")
                        }
                        onRowClick={(row) => {
                            navigate(
                                $path("/workflows/:workflow", {
                                    workflow: row.id,
                                })
                            );
                        }}
                    />
                </div>
            </div>
        </div>
    );

    function getColumns(): ColumnDef<Workflow, string>[] {
        return [
            columnHelper.accessor("name", {
                header: "Name",
            }),
            columnHelper.accessor("description", {
                header: "Description",
            }),
            columnHelper.accessor(
                (workflow) => threadCounts[workflow.id]?.toString(),
                {
                    id: "threads-action",
                    header: "Threads",
                    cell: (info) => (
                        <div className="flex gap-2 items-center">
                            <Button
                                asChild
                                variant="link"
                                className="underline"
                            >
                                <Link
                                    to={$path("/threads", {
                                        workflowId: info.row.original.id,
                                        from: "workflows",
                                    })}
                                    className="px-0"
                                >
                                    <TypographyP>
                                        {info.getValue() || 0} Threads
                                    </TypographyP>
                                </Link>
                            </Button>
                        </div>
                    ),
                }
            ),
            columnHelper.accessor("created", {
                id: "created",
                header: "Created",
                cell: (info) => (
                    <TypographyP>
                        {timeSince(new Date(info.row.original.created))} ago
                    </TypographyP>
                ),
            }),
            columnHelper.display({
                id: "actions",
                cell: ({ row }) => (
                    <div className="flex gap-2 justify-end">
                        <WorkflowViewYaml workflow={row.original} />

                        <Tooltip>
                            <TooltipTrigger asChild>
                                <Button variant="ghost" size="icon" asChild>
                                    <Link
                                        to={$path("/workflows/:workflow", {
                                            workflow: row.original.id,
                                        })}
                                    >
                                        <PenSquareIcon />
                                    </Link>
                                </Button>
                            </TooltipTrigger>

                            <TooltipContent>Edit Workflow</TooltipContent>
                        </Tooltip>

                        <DeleteWorkflowButton id={row.original.id} />
                    </div>
                ),
            }),
        ];
    }
}

const columnHelper = createColumnHelper<Workflow>();
