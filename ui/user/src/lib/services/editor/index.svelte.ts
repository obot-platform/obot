import { type EditorItem } from '$lib/stores/editor.svelte';
import editorStore from '$lib/stores/editor.svelte';
import tasks from '$lib/stores/tasks.svelte';
import ChatService from '../chat';
import { type Writable, writable } from 'svelte/store';

const visible = writable(false);
const maxSize = writable(false);
const items = editorStore.items;

const editor: Editor = {
	remove,
	load,
	download,
	select,
	items,
	maxSize,
	visible
};

export interface Editor {
	load: (
		id: string,
		opts?: {
			taskID?: string;
			runID?: string;
		}
	) => Promise<void>;
	download: (
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
	if (id.startsWith('tl1')) {
		await genericLoad(id);
		visible.set(true);
		return;
	}
	if (id.startsWith('w1')) {
		await loadTask(id);
		visible.set(true);
		return;
	}
	if (id.startsWith('table://')) {
		await loadTable(id);
		visible.set(true);
		return;
	}
	await loadFile(id, opts);
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

async function genericLoad(id: string) {
	const targetFile: EditorItem = {
		id: id,
		name: id,
		generic: true,
		contents: '',
		buffer: ''
	};
	items.push(targetFile);
	select(id);
}

async function loadTask(taskID: string) {
	try {
		let task = tasks.items.find((task) => task.id === taskID);
		if (!task) {
			task = await ChatService.getTask(taskID);
			tasks.items.push(task);
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

async function download(id: string, opts?: { taskID?: string; runID?: string }) {
	const item = items.find((item) => item.id === id);
	if (item && item.modified && item.buffer) {
		await ChatService.saveContents(item.id, item.buffer, opts);
		item.contents = item.buffer;
		item.modified = false;
		item.blob = undefined;
	}
	await ChatService.download(id, opts);
}

async function loadFile(
	file: string,
	opts?: {
		taskID?: string;
		runID?: string;
	}
) {
	try {
		const blob = await ChatService.getFile(file, opts);
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
		for (let i = 0; i < items.length; i++) {
			if (items[i].id === targetFile.id) {
				items[i] = targetFile;
				select(targetFile.id);
				return;
			}
		}
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
