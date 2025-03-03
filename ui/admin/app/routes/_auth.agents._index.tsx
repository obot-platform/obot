import { PlusIcon } from "@radix-ui/react-icons";
import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { SquarePen } from "lucide-react";
import { useMemo } from "react";
import { MetaFunction } from "react-router";
import { $path } from "safe-routes";
import useSWR, { preload } from "swr";

import { Agent } from "~/lib/model/agents";
import { CapabilityTool } from "~/lib/model/toolReferences";
import { AgentService } from "~/lib/service/api/agentService";
import { TaskService } from "~/lib/service/api/taskService";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { generateRandomName } from "~/lib/service/nameGenerator";
import { timeSince } from "~/lib/utils";

import { DeleteAgent } from "~/components/agent/DeleteAgent";
import { DataTable, useRowNavigate } from "~/components/composed/DataTable";
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
		preload(...ThreadsService.getThreads.swr({})),
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
	const navigate = useRowNavigate((agent: Agent) =>
		$path("/agents/:id", { id: agent.id })
	);
	const getThreads = useSWR(...ThreadsService.getThreads.swr({}));
	const getTasks = useSWR(...TaskService.getTasks.swr({}), {
		fallbackData: [],
	});
	const threadsMap = useMemo(
		() => new Map(getThreads.data?.map((thread) => [thread.id, thread])),
		[getThreads.data]
	);
	const threadCounts = useMemo(() => {
		if (!getThreads.data) return {};
		return getThreads.data.reduce(
			(acc, thread) => {
				if (!thread.agentID) return acc;
				acc[thread.agentID] = (acc[thread.agentID] || 0) + 1;
				return acc;
			},
			{} as Record<string, number>
		);
	}, [getThreads.data]);

	const taskCounts = useMemo(() => {
		if (!getTasks.data) return {};

		return getTasks.data.reduce(
			(acc, task) => {
				const agentId = threadsMap.get(task.threadID)?.agentID;
				if (!agentId) return acc;
				acc[agentId] = (acc[agentId] || 0) + 1;
				return acc;
			},
			{} as Record<string, number>
		);
	}, [getTasks.data, threadsMap]);

	const getAgents = useSWR(...AgentService.getAgents.swr({}));

	const agents = useMemo(() => {
		return (
			getAgents.data
				?.filter((agent) => !agent.deleted)
				.map((agent) => ({
					...agent,
					threadCount: threadCounts[agent.id] || 0,
					taskCount: taskCounts[agent.id] || 0,
				})) ?? []
		);
	}, [getAgents.data, threadCounts, taskCounts]);

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
									navigate.internal(agent);
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
						onRowClick={navigate.internal}
						onCtrlClick={navigate.external}
					/>
				</div>
			</div>
		</div>
	);

	function getColumns(): ColumnDef<(typeof agents)[0], string>[] {
		return [
			columnHelper.accessor("name", {
				header: "Name",
			}),
			columnHelper.accessor("description", {
				header: "Description",
			}),
			columnHelper.accessor((agent) => agent.threadCount.toString(), {
				id: "threads-action",
				header: "Threads",
				cell: (info) => (
					<div className="flex items-center gap-2">
						<Link
							to={$path("/chat-threads", {
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
			columnHelper.accessor((agent) => agent.taskCount.toString(), {
				id: "tasks-action",
				header: "Tasks",
				cell: (info) => (
					<div className="flex items-center gap-2">
						<Link
							to={$path("/tasks", {
								agentId: info.row.original.id,
								from: "agents",
							})}
							className="px-0"
						>
							{info.getValue() || 0} Tasks
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
									to={$path("/agents/:id", {
										id: row.original.id,
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

const columnHelper = createColumnHelper<
	Agent & {
		threadCount: number;
		taskCount: number;
	}
>();

export const meta: MetaFunction = () => {
	return [{ title: "Obot • Agents" }];
};
