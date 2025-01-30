import { createTask, deleteTask, listTasks, saveTask } from '$lib/services/chat/operations';
import { type Task } from '$lib/services/chat/types';

async function reload() {
	store.items = (await listTasks()).items;
}

async function remove(id: string) {
	await deleteTask(id);
	await reload();
}

async function create(): Promise<Task> {
	const task = await createTask({
		id: '',
		name: 'New Task',
		steps: []
	});
	store.items.push(task);
	return task;
}

async function update(task: Task) {
	const newTask = await saveTask(task);
	await reload();
	return newTask;
}

const store = $state({
	items: [] as Task[],
	reload,
	remove,
	create,
	update
});

export default store;
