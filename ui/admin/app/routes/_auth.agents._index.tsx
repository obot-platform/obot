import { PlusIcon } from "@radix-ui/react-icons";
import { Link, useNavigate } from "@remix-run/react";
import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { SquarePen, Trash } from "lucide-react";
import { useMemo } from "react";
import { $path } from "remix-routes";
import useSWR from "swr";

import { Agent } from "~/lib/model/agents";
import { AgentService } from "~/lib/service/api/agentService";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { timeSince } from "~/lib/utils";

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

export default function Threads() {
    const navigate = useNavigate();
    const getThreads = useSWR(ThreadsService.getThreads.key(), () =>
        ThreadsService.getThreads()
    );

    const threadCounts = useMemo(() => {
        if (!getThreads.data) return {};
        return getThreads.data.reduce(
            (acc, thread) => {
                acc[thread.agentID ?? thread.workflowID] =
                    (acc[thread.agentID ?? thread.workflowID] || 0) + 1;
                return acc;
            },
            {} as Record<string, number>
        );
    }, [getThreads.data]);

    const getAgents = useSWR(
        AgentService.getAgents.key(),
        AgentService.getAgents
    );

    const agents = getAgents.data || [];

    const deleteAgent = useAsync(AgentService.deleteAgent, {
        onSuccess: () => {
            AgentService.revalidateAgents();
            ThreadsService.revalidateThreads();
        },
    });

    return (
        <div>
            <div className="h-full p-8 flex flex-col gap-4">
                <div className="flex-auto overflow-hidden">
                    <div className="flex space-x-2 width-full justify-end mb-8">
                        <Button
                            variant="outline"
                            className="justify-start"
                            onClick={() => {
                                AgentService.createAgent({
                                    agent: {} as Agent,
                                }).then((agent) => {
                                    navigate(
                                        $path("/agents/:agent", {
                                            agent: agent.id,
                                        })
                                    );
                                });
                            }}
                        >
                            <PlusIcon className="w-4 h-4 mr-2" />
                            New Agent
                        </Button>
                    </div>

                    <DataTable
                        columns={getColumns()}
                        data={agents}
                        sort={[{ id: "created", desc: true }]}
                    />
                </div>
            </div>
        </div>
    );

    function getColumns(): ColumnDef<Agent, string>[] {
        return [
            columnHelper.accessor("name", {
                header: "Name",
            }),
            columnHelper.accessor("description", {
                header: "Description",
            }),
            columnHelper.accessor(
                (agent) => threadCounts[agent.id]?.toString(),
                {
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
                                        agentId: info.row.original.id,
                                    })}
                                    className="px-0"
                                >
                                    <TypographyP>
                                        {info.getValue()} Threads
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
                        <TooltipProvider>
                            <Tooltip>
                                <TooltipTrigger asChild>
                                    <Button variant="ghost" size="icon" asChild>
                                        <Link
                                            to={$path("/agents/:agent", {
                                                agent: row.original.id,
                                            })}
                                        >
                                            <SquarePen />
                                        </Link>
                                    </Button>
                                </TooltipTrigger>

                                <TooltipContent>
                                    <p>Edit Agent</p>
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
                                            deleteAgent.execute(row.original.id)
                                        }
                                    >
                                        <Trash />
                                    </Button>
                                </TooltipTrigger>

                                <TooltipContent>
                                    <p>Delete Agent</p>
                                </TooltipContent>
                            </Tooltip>
                        </TooltipProvider>
                    </div>
                ),
            }),
        ];
    }
}

const columnHelper = createColumnHelper<Agent>();
