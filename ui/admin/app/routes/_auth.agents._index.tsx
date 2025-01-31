import { PlusIcon } from "@radix-ui/react-icons";
import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { SquarePen } from "lucide-react";
import { useMemo } from "react";
import { MetaFunction, useNavigate } from "react-router";
import { $path } from "safe-routes";
import useSWR, { preload } from "swr";

import { Agent } from "~/lib/model/agents";
import { CapabilityTool } from "~/lib/model/toolReferences";
import { AgentService } from "~/lib/service/api/agentService";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { generateRandomName } from "~/lib/service/nameGenerator";
import { timeSince } from "~/lib/utils";

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
		preload(...AgentService.getAgents.swr({})),
		preload(ThreadsService.getThreads.key(), ThreadsService.getThreads),
	]);
	return null;
}

const CapabilityTools = [
	CapabilityTool.Knowledge,
	CapabilityTool.WorkspaceFiles,
	CapabilityTool.Database,
	CapabilityTool.Tasks,
];
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

	const getAgents = useSWR(...AgentService.getAgents.swr({}));

	const agents = getAgents.data || [];

	return (
		<div>
			<div className="flex h-full flex-col gap-4 p-8">
				<div className="flex-auto overflow-hidden">
					<div className="width-full mb-8 flex justify-between space-x-2">
						<h2>Agents</h2>
						<Button
							variant="outline"
							className="justify-start"
							onClick={() => {
								AgentService.createAgent({
									agent: {
										name: generateRandomName(),
										tools: CapabilityTools,
									} as Agent,
								}).then((agent) => {
									getAgents.mutate();
									navigate(
										$path("/agents/:agent", {
											agent: agent.id,
										})
									);
								});
							}}
						>
							<PlusIcon className="mr-2 h-4 w-4" />
							New Agent
						</Button>
					</div>

					<DataTable
						columns={getColumns()}
						data={agents}
						sort={[{ id: "created", desc: true }]}
						disableClickPropagation={(cell) => cell.id.includes("action")}
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
			columnHelper.accessor((agent) => threadCounts[agent.id]?.toString(), {
				id: "threads-action",
				header: "Threads",
				cell: (info) => (
					<div className="flex items-center gap-2">
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
			}),
			columnHelper.accessor("created", {
				id: "created",
				header: "Created",
				cell: (info) => (
					<p>{timeSince(new Date(info.row.original.created))} ago</p>
				),
			}),
			columnHelper.display({
				id: "actions",
				cell: ({ row }) => (
					<div className="flex justify-end gap-2">
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
