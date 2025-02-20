import {
	ReactNode,
	createContext,
	useCallback,
	useContext,
	useState,
} from "react";
import useSWR, { mutate } from "swr";

import { Task } from "~/lib/model/tasks";
import { TaskService } from "~/lib/service/api/taskService";

import { useAsync } from "~/hooks/useAsync";

interface TaskContextType {
	task: Task;
	taskId: string;
	updateTask: (task: Task) => void;
	refreshTask: () => void;
	isUpdating: boolean;
	lastUpdated?: Date;
}

const TaskContext = createContext<TaskContextType | undefined>(undefined);

export function TaskProvider({
	children,
	task: initialTask,
}: {
	children: ReactNode;
	task: Task;
}) {
	const taskId = initialTask.id;

	const getTask = useSWR(...TaskService.getTaskById.swr({ taskId }), {
		fallbackData: initialTask,
	});

	const [lastUpdated, setLastSaved] = useState<Date>();

	const handleUpdateTask = useCallback(
		(updatedTask: Task) =>
			TaskService.updateTask({
				id: taskId,
				task: updatedTask,
			})
				.then((updatedTask) => {
					getTask.mutate(updatedTask);
					mutate(TaskService.getTasks.key());
					setLastSaved(new Date());
				})
				.catch(console.error),
		[taskId, getTask]
	);

	const updateTask = useAsync(handleUpdateTask);

	const refreshTask = getTask.mutate;

	return (
		<TaskContext.Provider
			value={{
				taskId,
				task: getTask.data,
				updateTask: updateTask.execute,
				refreshTask,
				isUpdating: updateTask.isLoading,
				lastUpdated,
			}}
		>
			{children}
		</TaskContext.Provider>
	);
}

export function useTask() {
	const context = useContext(TaskContext);
	if (context === undefined) {
		throw new Error("useTask must be used within a TaskProvider");
	}
	return context;
}
