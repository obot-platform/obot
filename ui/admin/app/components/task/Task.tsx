import { List, PuzzleIcon } from "lucide-react";
import { useCallback, useState } from "react";

import { Task as TaskType, UpdateTask } from "~/lib/model/tasks";
import { cn } from "~/lib/utils";

import { AgentForm } from "~/components/agent";
import { ParamsForm } from "~/components/task/ParamsForm";
import { TaskProvider, useTask } from "~/components/task/TaskContext";
import { StepsForm } from "~/components/task/steps/StepsForm";
import { TaskTriggerPanel } from "~/components/task/triggers/TaskTriggerPanel";
import { ScrollArea } from "~/components/ui/scroll-area";
import { useDebounce } from "~/hooks/useDebounce";

type TaskProps = {
	task: TaskType;
	onPersistThreadId: (threadId: string) => void;
	className?: string;
};

export function Task(props: TaskProps) {
	return (
		<TaskProvider task={props.task}>
			<TaskContent {...props} />
		</TaskProvider>
	);
}

function TaskContent({ className }: TaskProps) {
	const { task, updateTask, isUpdating, lastUpdated } = useTask();

	const [taskUpdates, setTaskUpdates] = useState(task);

	const debouncedUpdateTask = useDebounce(updateTask, 1000);

	const partialSetTask = useCallback(
		(changes: Partial<UpdateTask>) => {
			const updatedTask = {
				...task,
				...taskUpdates,
				...changes,
			};

			debouncedUpdateTask(updatedTask);

			setTaskUpdates(updatedTask);
		},
		[debouncedUpdateTask, task, taskUpdates]
	);

	return (
		<div className="flex h-full flex-col">
			<ScrollArea className={cn("h-full", className)}>
				<div className="m-4 px-4 pb-4">
					<AgentForm
						agent={taskUpdates}
						onChange={partialSetTask}
						hideImageField
						hideInstructionsField
					/>
				</div>

				<div className="m-4 flex flex-col gap-4 p-4">
					<h4 className="flex items-center gap-2">
						<List />
						Arguments
					</h4>

					<ParamsForm
						task={task}
						onChange={(values) => {
							partialSetTask({
								params: values.params,
							});
						}}
					/>
				</div>

				<div className="m-4 flex flex-col gap-4 p-4">
					<h4 className="flex items-center gap-2">
						<PuzzleIcon />
						Steps
					</h4>

					<StepsForm
						task={taskUpdates}
						onChange={(values) => partialSetTask({ steps: values.steps })}
					/>
				</div>

				<TaskTriggerPanel taskId={task.id} />
			</ScrollArea>

			<footer className="flex items-center justify-between gap-4 border-t p-4 text-muted-foreground">
				{isUpdating ? <p>Saving...</p> : lastUpdated ? <p>Saved</p> : <div />}
			</footer>
		</div>
	);
}
