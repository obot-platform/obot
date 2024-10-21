import { ReaderIcon } from "@radix-ui/react-icons";
import {
    ClientLoaderFunctionArgs,
    Link,
    useLoaderData,
} from "@remix-run/react";
import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { Trash } from "lucide-react";
import { useMemo } from "react";
import { $path } from "remix-routes";
import useSWR from "swr";
import { z } from "zod";

import { Agent } from "~/lib/model/agents";
import { Thread } from "~/lib/model/threads";
import { Workflow } from "~/lib/model/workflows";
import { AgentService } from "~/lib/service/api/agentService";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { WorkflowService } from "~/lib/service/api/workflowService";
import { QueryParamSchemas } from "~/lib/service/routeQueryParams";
import { parseQueryParams, timeSince } from "~/lib/utils";

import { TypographyP } from "~/components/Typography";
import { DataTable } from "~/components/composed/DataTable";
import { Button } from "~/components/ui/button";
import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from "~/components/ui/tooltip";
import { useAsync } from "~/hooks/useAsync";

const paramSchema = QueryParamSchemas.Threads;

export type SearchParams = z.infer<typeof paramSchema>;

export function clientLoader({ request }: ClientLoaderFunctionArgs) {
    return parseQueryParams(request.url, paramSchema).data || {};
}

export default function Threads() {
    const { agentId, workflowId } = useLoaderData<typeof clientLoader>();

    const getThreads = useSWR(
        ThreadsService.getThreads.key(),
        ThreadsService.getThreads
    );

    const getAgents = useSWR(
        AgentService.getAgents.key(),
        AgentService.getAgents
    );

    const getWorkflows = useSWR(
        WorkflowService.getWorkflows.key(),
        WorkflowService.getWorkflows
    );

    const agentMap = useMemo(() => {
        if (!getAgents.data) return {};

        return getAgents.data.reduce(
            (acc, agent) => {
                acc[agent.id] = agent;
                return acc;
            },
            {} as Record<string, Agent>
        );
    }, [getAgents.data]);

    const workflowMap = useMemo(() => {
        if (!getWorkflows.data) return {};

        return getWorkflows.data.reduce(
            (acc, workflow) => {
                acc[workflow.id] = workflow;
                return acc;
            },
            {} as Record<string, Workflow>
        );
    }, [getWorkflows.data]);

    const threads = useMemo(() => {
        if (!getThreads.data) return [];

        if (!agentId && !workflowId) return getThreads.data;

        switch (true) {
            case !!agentId:
                return getThreads.data.filter(
                    (thread) => thread.agentID === agentId
                );
            case !!workflowId:
                return getThreads.data.filter(
                    (thread) => thread.workflowID === workflowId
                );
            default:
                return getThreads.data;
        }
    }, [getThreads.data, agentId, workflowId]);

    const deleteThread = useAsync(ThreadsService.deleteThread, {
        onSuccess: ThreadsService.revalidateThreads,
    });

    return (
        <div className="h-full flex flex-col">
            <div className="flex-auto overflow-hidden p-8">
                <DataTable
                    columns={getColumns()}
                    data={threads}
                    sort={[{ id: "created", desc: true }]}
                    classNames={{
                        row: "!max-h-[200px] grow-0  height-[200px]",
                        cell: "!max-h-[200px] grow-0 height-[200px]",
                    }}
                />
            </div>
        </div>
    );

    function getColumns(): ColumnDef<Thread, string>[] {
        return [
            columnHelper.accessor(
                (thread) => {
                    if (thread.agentID)
                        return agentMap[thread.agentID]?.name ?? thread.agentID;
                    if (thread.workflowID)
                        return (
                            workflowMap[thread.workflowID]?.name ??
                            thread.workflowID
                        );
                    return "Unnamed";
                },
                { header: "Agent" }
            ),
            columnHelper.accessor("created", {
                id: "created",
                header: "Created",
                cell: (info) => (
                    <TypographyP>
                        {timeSince(new Date(info.row.original.created))} ago
                    </TypographyP>
                ),
                sortingFn: "datetime",
            }),
            columnHelper.display({
                id: "actions",
                cell: ({ row }) => (
                    <div className="flex gap-2 justify-end">
                        <TooltipProvider>
                            <Tooltip>
                                <TooltipTrigger asChild>
                                    <Button variant="ghost" size="icon" asChild>
                                        <Link
                                            to={$path("/thread/:id", {
                                                id: row.original.id,
                                            })}
                                        >
                                            <ReaderIcon
                                                width={21}
                                                height={21}
                                            />
                                        </Link>
                                    </Button>
                                </TooltipTrigger>

                                <TooltipContent>
                                    <p>Inspect Thread</p>
                                </TooltipContent>
                            </Tooltip>
                        </TooltipProvider>

                        <TooltipProvider>
                            <Tooltip>
                                <TooltipTrigger asChild>
                                    <Button
                                        variant="ghost"
                                        size="icon"
                                        onClick={() =>
                                            deleteThread.execute(
                                                row.original.id
                                            )
                                        }
                                    >
                                        <Trash />
                                    </Button>
                                </TooltipTrigger>

                                <TooltipContent>
                                    <p>Delete Thread</p>
                                </TooltipContent>
                            </Tooltip>
                        </TooltipProvider>
                    </div>
                ),
            }),
        ];
    }
}

const columnHelper = createColumnHelper<Thread>();
