import { $path } from "safe-routes";

import { Agent } from "~/lib/model/agents";
import { Project } from "~/lib/model/project";
import { Task } from "~/lib/model/tasks";
import { pluralize } from "~/lib/utils";

import { Card, CardContent } from "~/components/ui/card";
import { Link } from "~/components/ui/link";

type TaskMetaProps = {
	task: Task;
	agent: Agent;
	project: Project;
	taskRuns: number;
};

export function TaskMeta({ task, agent, project, taskRuns }: TaskMetaProps) {
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
							<tr className="border-foreground/25">
								<td className="py-2 pr-4 font-medium">Obot</td>
								<td className="text-right">
									<Link
										to={$path("/obots", {
											obotId: project.id,
											showChildren: true,
										})}
									>
										{project.name}
									</Link>
								</td>
							</tr>
							<tr className="border-foreground/25">
								<td className="py-2 pr-4 font-medium">Agent</td>
								<td className="text-right">
									<Link to={$path("/agents/:id", { id: agent.id })}>
										{agent.name}
									</Link>
								</td>
							</tr>
							<tr className="border-foreground/25">
								<td className="py-2 pr-4 font-medium">Task Runs</td>
								<td className="text-right">
									<Link to={$path("/task-runs", { taskId: task.id })}>
										{taskRuns} {pluralize(taskRuns, "Task Run")}
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
