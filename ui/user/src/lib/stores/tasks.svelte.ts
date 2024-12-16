import { createTask, deleteTask, listTasks, saveTask } from '$lib/services/chat/operations';
import { type Task } from '$lib/services/chat/types';
import { currentAssistant } from '$lib/stores/index';
import { SvelteMap } from 'svelte/reactivity';
import { get } from 'svelte/store';

const items = $state(new SvelteMap<string, Task>());

async function reload() {
	const assistantID = get(currentAssistant)?.id;
	if (!assistantID) {
		return;
	}
	const tasks = await listTasks(assistantID);
	items.clear();
	for (const task of tasks.items) {
		items.set(task.id, task);
	}
}

async function remove(id: string) {
	const assistantID = get(currentAssistant)?.id;
	if (!assistantID) {
		return;
	}
	await deleteTask(assistantID, id);
	items.delete(id);
}

async function update(task: Task): Promise<Task> {
	const assistantID = get(currentAssistant)?.id;
	if (!assistantID) {
		return task;
	}
	const newTask = await saveTask(assistantID, task);
	items.set(newTask.id, newTask);
	return newTask;
}

async function create(): Promise<Task> {
	const assistantID = get(currentAssistant)?.id;
	if (!assistantID) {
		throw new Error('No assistant selected');
	}
	const task = await createTask(assistantID, {
		id: '',
		name: 'New Task',
		steps: []
	});
	items.set(task.id, task);
	return task;
}

export interface TaskStore {
	items: Map<string, Task>;
	reload: () => Promise<void>;
	update: (task: Task) => Promise<Task>;
	remove: (id: string) => Promise<void>;
	create: () => Promise<Task>;
}

const store: TaskStore = {
	items,
	reload,
	remove,
	create,
	update
};

export default store;
