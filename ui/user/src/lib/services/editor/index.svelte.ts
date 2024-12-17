import items, { type EditorItem } from '$lib/stores/editor.svelte';
import tasks from '$lib/stores/tasks.svelte';
import ChatService from '../chat';
import { type Writable, writable } from 'svelte/store';

const visible = writable(false);
const maxSize = writable(false);

const editor: Editor = {
	remove,
	load,
	select,
	items,
	maxSize,
	visible
};

export interface Editor {
	load: (
		assistant: string,
		id: string,
		opts?: {
			taskID?: string;
			runID?: string;
		}
	) => Promise<void>;
	remove: (name: string) => void;
	select: (name: string) => void;
	items: EditorItem[];
	visible: Writable<boolean>;
	maxSize: Writable<boolean>;
}

function hasItem(id: string): boolean {
	const item = items?.find((item) => item.id === id);
	return item !== undefined;
}

async function load(
	assistant: string,
	id: string,
	opts?: {
		taskID?: string;
		runID?: string;
	}
) {
	let fileID = id;
	if (opts?.taskID && opts?.runID) {
		fileID = `${opts.taskID}/${opts.runID}/${id}`;
	}
	if (hasItem(fileID)) {
		select(fileID);
		visible.set(true);
		return;
	}
	if (id.startsWith('w1')) {
		await loadTask(assistant, id);
		visible.set(true);
		return;
	}
	if (id.startsWith('table://')) {
		await loadTable(id);
		visible.set(true);
		return;
	}
	await loadFile(assistant, id, opts);
	visible.set(true);
}

async function loadTable(id: string) {
	const tableName = id.split('table://')[1];
	const targetFile: EditorItem = {
		id: id,
		name: tableName,
		contents: '',
		buffer: '',
		modified: false,
		selected: true,
		table: tableName
	};
	items.push(targetFile);
	select(id);
}

async function loadTask(assistant: string, taskID: string) {
	try {
		let task = tasks.items.get(taskID);
		if (!task) {
			task = await ChatService.getTask(assistant, taskID);
			tasks.items.set(taskID, task);
		}
		const targetFile: EditorItem = {
			id: taskID,
			name: task.name ?? `Task ${taskID}`,
			contents: '',
			buffer: '',
			modified: false,
			selected: true,
			task
		};
		items.push(targetFile);
		select(task.id);
	} catch {
		// ignore error
	}
}

async function loadFile(
	assistant: string,
	file: string,
	opts?: {
		taskID?: string;
		runID?: string;
	}
) {
	try {
		const blob = await ChatService.getFile(assistant, file, opts);
		const contents = await blob.text();
		let fileID = file;
		if (opts?.taskID && opts?.runID) {
			fileID = `${opts.taskID}/${opts.runID}/${file}`;
		}
		const targetFile = {
			id: fileID,
			taskID: opts?.taskID,
			runID: opts?.runID,
			name: file,
			contents,
			blob: blob,
			buffer: '',
			modified: false,
			selected: true
		};
		items.push(targetFile);
		select(targetFile.id);
	} catch {
		// ignore error
	}
}

function select(id: string) {
	if (!id) {
		return;
	}

	let matched = false;
	for (const item of items) {
		if (item.id === id) {
			item.selected = true;
			matched = true;
		} else {
			item.selected = false;
		}
	}

	if (!matched && items.length > 0) {
		items[0].selected = true;
	}
}

function remove(id: string) {
	for (let i = 0; i < items.length; i++) {
		if (items[i].id === id) {
			if (i > 0) {
				select(items[i - 1].id);
			} else if (items.length > 1) {
				select(items[i + 1].id);
			}
			items.splice(i, 1);
			break;
		}
	}

	if (items.length === 0) {
		visible.set(false);
	}
}

export default editor;
