import { $path } from "safe-routes";
import useSWR from "swr";

import { Task } from "~/lib/model/tasks";
import { AgentService } from "~/lib/service/api/agentService";
import { ThreadsService } from "~/lib/service/api/threadsService";

import { Card, CardContent } from "~/components/ui/card";
import { Link } from "~/components/ui/link";

export function TaskMeta({ task }: { task: Task }) {
	const getThreads = useSWR(...ThreadsService.getThreads.swr({}));
	const getAgents = useSWR(...AgentService.getAgents.swr({}));

	const taskRuns = getThreads?.data?.filter(
		(thread) => thread.workflowID === task.id
	);
	const rootThread = getThreads?.data?.find(
		(thread) => thread.id === task.threadID
	);
	const agent = getAgents?.data?.find(
		(agent) => agent.id === rootThread?.agentID
	);

	return (
		<Card className="bg-0 h-full overflow-hidden">
			<CardContent className="space-y-4 pt-6">
				<div className="overflow-hidden rounded-md bg-muted p-4">
					<table className="w-full">
						<tbody>
							<tr className="border-foreground/25">
								<td className="py-2 pr-4 font-medium">Created</td>
								<td className="text-right">
									{new Date(task.created).toLocaleString()}
								</td>
							</tr>
							{agent && (
								<tr className="border-foreground/25">
									<td className="py-2 pr-4 font-medium">Agent</td>
									<td className="text-right">
										<Link to={$path("/agents/:id", { id: agent.id })}>
											{agent.name}
										</Link>
									</td>
								</tr>
							)}
							<tr className="border-foreground/25">
								<td className="py-2 pr-4 font-medium">Task Runs</td>
								<td className="text-right">
									<Link to={$path("/task-runs", { taskId: task.id })}>
										{taskRuns?.length ?? 0} Task Runs
									</Link>
								</td>
							</tr>
						</tbody>
					</table>
				</div>
			</CardContent>
		</Card>
	);
}
