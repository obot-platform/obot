import { PlusIcon } from "@radix-ui/react-icons";
import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { SquarePen } from "lucide-react";
import { useMemo } from "react";
import { MetaFunction, useNavigate } from "react-router";
import { $path } from "safe-routes";
import useSWR, { mutate, preload } from "swr";

import { Agent } from "~/lib/model/agents";
import { AgentService } from "~/lib/service/api/agentService";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { generateRandomName } from "~/lib/service/nameGenerator";
import { timeSince } from "~/lib/utils";

import { TypographyH2, TypographyP } from "~/components/Typography";
import { DeleteAgent } from "~/components/agent/DeleteAgent";
import { DataTable } from "~/components/composed/DataTable";
import { Button } from "~/components/ui/button";
import { Link } from "~/components/ui/link";
import {
    Tooltip,
    TooltipContent,
    TooltipTrigger,
} from "~/components/ui/tooltip";

export async function clientLoader() {
    await Promise.all([
        preload(AgentService.getAgents.key(), AgentService.getAgents),
        preload(ThreadsService.getThreads.key(), ThreadsService.getThreads),
    ]);
    return null;
}

export default function Agents() {
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

    return (
        <div>
            <div className="h-full p-8 flex flex-col gap-4">
                <div className="flex-auto overflow-hidden">
                    <div className="flex space-x-2 width-full justify-between mb-8">
                        <TypographyH2>Agents</TypographyH2>
                        <Button
                            variant="outline"
                            className="justify-start"
                            onClick={() => {
                                AgentService.createAgent({
                                    agent: {
                                        name: generateRandomName(),
                                    } as Agent,
                                }).then((agent) => {
                                    mutate(AgentService.getAgents.key());
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
                        disableClickPropagation={(cell) =>
                            cell.id.includes("action")
                        }
                        onRowClick={(row) => {
                            navigate(
                                $path("/agents/:agent", {
                                    agent: row.id,
                                })
                            );
                        }}
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
                    id: "threads-action",
                    header: "Threads",
                    cell: (info) => (
                        <div className="flex gap-2 items-center">
                            <Link
                                to={$path("/threads", {
                                    agentId: info.row.original.id,
                                    from: "agents",
                                })}
                                className="px-0"
                            >
                                {info.getValue() || 0} Threads
                            </Link>
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
                        <Tooltip>
                            <TooltipTrigger asChild>
                                <Link
                                    to={$path("/agents/:agent", {
                                        agent: row.original.id,
                                    })}
                                    as="button"
                                    size="icon"
                                    variant="ghost"
                                >
                                    <SquarePen />
                                </Link>
                            </TooltipTrigger>

                            <TooltipContent>
                                <p>Edit Agent</p>
                            </TooltipContent>
                        </Tooltip>

                        <DeleteAgent id={row.original.id} />
                    </div>
                ),
            }),
        ];
    }
}

const columnHelper = createColumnHelper<Agent>();

export const meta: MetaFunction = () => {
    return [{ title: "Obot • Agents" }];
};
