import { useMemo } from "react";
import { $path } from "safe-routes";
import useSWR from "swr";

import { Agent } from "~/lib/model/agents";
import { TaskService } from "~/lib/service/api/taskService";
import { ThreadsService } from "~/lib/service/api/threadsService";

import { Card, CardContent } from "~/components/ui/card";
import { Link } from "~/components/ui/link";

export function AgentMeta({ agent }: { agent: Agent }) {
	const { data: threads } = useSWR(
		...ThreadsService.getThreadsByAgent.swr({ agentId: agent.id })
	);

	const threadsMap = useMemo(
		() => new Map(threads?.map((thread) => [thread.id, thread])),
		[threads]
	);

	const { data: tasks } = useSWR(...TaskService.getTasks.swr({}));
	const agentTasks = tasks?.filter((task) => threadsMap.get(task.threadID));

	return (
		<Card className="bg-0 h-full overflow-hidden">
			<CardContent className="space-y-4 pt-6">
				<div className="overflow-hidden rounded-md bg-muted p-4">
					<table className="w-full">
						<tbody>
							<tr className="border-foreground/25">
								<td className="py-2 pr-4 font-medium">Created</td>
								<td className="text-right">
									{new Date(agent.created).toLocaleString()}
								</td>
							</tr>
							<tr className="border-foreground/25">
								<td className="py-2 pr-4 font-medium">Threads</td>
								<td className="text-right">
									<Link to={$path("/chat-threads", { agentId: agent.id })}>
										{threads?.length ?? 0} Threads
									</Link>
								</td>
							</tr>
							<tr className="border-foreground/25">
								<td className="py-2 pr-4 font-medium">Tasks</td>
								<td className="text-right">
									<Link to={$path("/tasks", { agentId: agent.id })}>
										{agentTasks?.length ?? 0} Tasks
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
