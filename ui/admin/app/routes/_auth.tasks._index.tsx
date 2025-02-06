import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { useMemo, useState } from "react";
import { $path } from "safe-routes";
import useSWR, { preload } from "swr";

import { Workflow } from "~/lib/model/workflows";
import { AgentService } from "~/lib/service/api/agentService";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { WorkflowService } from "~/lib/service/api/workflowService";
import { RouteHandle } from "~/lib/service/routeHandles";

import { DataTable, useRowNavigate } from "~/components/composed/DataTable";
import { SearchInput } from "~/components/composed/SearchInput";
import { Link } from "~/components/ui/link";

export async function clientLoader() {
	await Promise.all([
		preload(WorkflowService.getWorkflows.key(), WorkflowService.getWorkflows),
		preload(ThreadsService.getThreads.key(), ThreadsService.getThreads),
		preload(...AgentService.getAgents.swr({})),
	]);
	return null;
}

export default function Tasks() {
	const [search, setSearch] = useState("");
	const { onRowClick, onCtrlClick } = useRowNavigate<Workflow>(
		"/tasks/:id",
		"id"
	);
	const getAgents = useSWR(...AgentService.getAgents.swr({}));
	const getThreads = useSWR(
		ThreadsService.getThreads.key(),
		ThreadsService.getThreads
	);
	const getWorkflows = useSWR(
		WorkflowService.getWorkflows.key(),
		WorkflowService.getWorkflows
	);

	const agentsMap = useMemo(() => {
		return new Map(getAgents.data?.map((agent) => [agent.id, agent]));
	}, [getAgents.data]);

	const userTasks: (Workflow & {
		agent: string | undefined;
		threadCount: number;
	})[] = useMemo(() => {
		const threadsMap = new Map(
			getThreads.data?.map((thread) => [thread.id, thread])
		);

		const threadCounts = getThreads.data?.reduce<Record<string, number>>(
			(acc, thread) => {
				if (!thread.workflowID) return acc;

				acc[thread.workflowID] = (acc[thread.workflowID] || 0) + 1;
				return acc;
			},
			{}
		);

		return (
			getWorkflows.data?.map((workflow) => ({
				...workflow,
				agent:
					agentsMap.get(threadsMap.get(workflow.threadID ?? "")?.agentID ?? "")
						?.name ?? "-",
				threadCount: threadCounts?.[workflow.id] || 0,
			})) ?? []
		);
	}, [getWorkflows.data, agentsMap, getThreads.data]);

	const itemsToDisplay = search
		? userTasks.filter(
				(item) =>
					item.name.toLowerCase().includes(search.toLowerCase()) ||
					item.agent?.toLowerCase().includes(search.toLowerCase())
			)
		: userTasks;

	return (
		<div>
			<div className="flex h-full flex-col gap-4 p-8">
				<div className="flex-auto overflow-hidden">
					<div className="flex items-center justify-between pb-8">
						<h2>Tasks</h2>
						<SearchInput
							onChange={(value) => setSearch(value)}
							placeholder="Search for tasks..."
						/>
					</div>

					<DataTable
						columns={getColumns()}
						data={itemsToDisplay}
						sort={[{ id: "created", desc: true }]}
						onRowClick={onRowClick}
						onCtrlClick={onCtrlClick}
					/>
				</div>
			</div>
		</div>
	);

	function getColumns(): ColumnDef<(typeof userTasks)[0], string>[] {
		return [
			columnHelper.accessor("name", {
				header: "Name",
			}),
			columnHelper.accessor("agent", {
				header: "Agent",
			}),
			columnHelper.accessor((item) => item.threadCount.toString(), {
				id: "tasks-action",
				header: "Runs",
				cell: (info) => (
					<div className="flex items-center gap-2">
						<Link
							onClick={(event) => event.stopPropagation()}
							to={$path("/task-runs", {
								taskId: info.row.original.id,
								from: "tasks",
							})}
							className="px-0"
						>
							<p>{info.getValue() || 0} Runs</p>
						</Link>
					</div>
				),
			}),
		];
	}
}

const columnHelper = createColumnHelper<
	Workflow & { agent: string | undefined; threadCount: number }
>();

export const handle: RouteHandle = {
	breadcrumb: () => [{ content: "Tasks" }],
};
