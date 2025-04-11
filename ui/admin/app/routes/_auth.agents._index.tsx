import { PlusIcon } from "@radix-ui/react-icons";
import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { SquarePen } from "lucide-react";
import { MetaFunction, useLoaderData } from "react-router";
import { $path } from "safe-routes";
import { preload } from "swr";

import { Agent } from "~/lib/model/agents";
import { CapabilityTool } from "~/lib/model/toolReferences";
import { AgentService } from "~/lib/service/api/agentService";
import { ProjectApiService } from "~/lib/service/api/projectApiService";
import { TaskService } from "~/lib/service/api/taskService";
import { generateRandomName } from "~/lib/service/nameGenerator";
import { pluralize, timeSince } from "~/lib/utils";

import { DefaultAgent } from "~/components/agent/DefaultAgent";
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
	const [baseAgents, tasks, projects] = await Promise.all([
		preload(...AgentService.getAgents.swr({})),
		preload(...TaskService.getTasks.swr({})),
		preload(...ProjectApiService.getAll.swr({})),
	]);

	const projectMap = new Map(projects.map((project) => [project.id, project]));

	const taskCounts = tasks.reduce<Record<string, number>>((acc, task) => {
		const agentId = projectMap.get(task.projectID)?.assistantID;
		if (!agentId) return acc;
		acc[agentId] = (acc[agentId] || 0) + 1;
		return acc;
	}, {});

	const projectCounts = projects.reduce<Record<string, number>>(
		(acc, { assistantID }) => {
			if (!assistantID) return acc;
			acc[assistantID] = (acc[assistantID] || 0) + 1;
			return acc;
		},
		{}
	);

	const agents = baseAgents
		.filter((agent) => !agent.deleted)
		.map((agent) => ({
			...agent,
			taskCount: taskCounts[agent.id] || 0,
			projectCount: projectCounts[agent.id] || 0,
		}));

	return { agents };
}

const CapabilityTools = [
	CapabilityTool.Knowledge,
	CapabilityTool.WorkspaceFiles,
	CapabilityTool.Database,
	CapabilityTool.Tasks,
];
export default function Agents() {
	const { agents } = useLoaderData<typeof clientLoader>();

	const navigate = useRowNavigate<Agent>(({ id }) =>
		$path("/agents/:id", { id })
	);

	return (
		<div>
			<div className="flex h-full flex-col gap-4 p-8">
				<div className="flex-auto overflow-hidden">
					<div className="width-full mb-8 flex justify-between space-x-2">
						<h2>Agents</h2>
						<div className="flex items-center gap-4">
							<DefaultAgent agents={agents} />
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
										AgentService.getAgents.revalidate();
										navigate.internal(agent);
									});
								}}
							>
								<PlusIcon className="h-4 w-4" />
								New Agent
							</Button>
						</div>
					</div>

					<DataTable
						columns={getColumns()}
						data={agents}
						sort={[{ id: "created", desc: true }]}
						onRowClick={navigate.internal}
						onCtrlClick={navigate.external}
					/>
				</div>
			</div>
		</div>
	);

	function getColumns(): ColumnDef<AgentWithCounts, string>[] {
		return [
			columnHelper.accessor("id", {
				header: "ID",
			}),
			columnHelper.accessor("name", {
				header: "Name",
			}),
			columnHelper.accessor("description", {
				header: "Description",
			}),
			columnHelper.accessor((agent) => agent.projectCount.toString(), {
				id: "projects",
				header: "Obots",
				cell: (info) => (
					<div className="flex items-center gap-2">
						<Link
							onClick={(e) => e.stopPropagation()}
							to={$path("/obots", {
								agentId: info.row.original.id,
								showChildren: true,
							})}
							className="px-0"
						>
							{info.getValue()} {pluralize(Number(info.getValue()), "Obot")}
						</Link>
					</div>
				),
			}),
			columnHelper.accessor((agent) => agent.taskCount.toString(), {
				id: "tasks",
				header: "Tasks",
				cell: (info) => (
					<div className="flex items-center gap-2">
						<Link
							onClick={(e) => e.stopPropagation()}
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
									onClick={(e) => e.stopPropagation()}
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

type AgentWithCounts = Awaited<
	ReturnType<typeof clientLoader>
>["agents"][number];

const columnHelper = createColumnHelper<AgentWithCounts>();

export const meta: MetaFunction = () => {
	return [{ title: "Obot • Agents" }];
};
