import type { Project, Task } from '$lib/services';
import ChatService from '../chat';

export interface EditorItem {
	id: string;
	name: string;
	file?: {
		contents: string;
		modified?: boolean;
		buffer: string;
		threadID?: string;
		blob?: Blob;
		taskID?: string;
		runID?: string;
	};
	task?: Task;
	table?: {
		name: string;
	};
	selected?: boolean;
	generic?: boolean;
}

function hasItem(items: EditorItem[], id: string): boolean {
	const item = items?.find((item) => item.id === id);
	return item !== undefined;
}

async function load(
	items: EditorItem[],
	project: Project,
	id: string,
	opts?: {
		taskID?: string;
		threadID?: string;
		runID?: string;
	}
) {
	let fileID = id;
	if (opts?.taskID && opts?.runID) {
		fileID = `${opts.taskID}/${opts.runID}/${id}`;
	} else if (opts?.threadID) {
		fileID = `${opts.threadID}/${id}`;
	}
	if (hasItem(items, fileID)) {
		select(items, fileID);
	} else if (id.startsWith('tl1')) {
		await genericLoad(items, id);
	} else if (id.startsWith('w1')) {
		await loadTask(items, project, id);
	} else if (id.startsWith('table://')) {
		await loadTable(items, id);
	} else {
		await loadFile(items, project, id, opts);
	}
}

async function loadTable(items: EditorItem[], id: string) {
	const tableName = id.split('table://')[1];
	const targetFile: EditorItem = {
		id: id,
		name: tableName,
		selected: true,
		table: {
			name: tableName
		}
	};
	items.push(targetFile);
	select(items, id);
}

async function genericLoad(items: EditorItem[], id: string) {
	const targetFile: EditorItem = {
		id: id,
		name: id,
		generic: true
	};
	items.push(targetFile);
	select(items, id);
}

async function loadTask(items: EditorItem[], project: Project, taskID: string) {
	try {
		const task = await ChatService.getTask(project.assistantID, project.id, taskID);
		const targetFile: EditorItem = {
			id: taskID,
			name: task.name ?? `Task ${taskID}`,
			task: task
		};
		items.push(targetFile);
		select(items, task.id);
	} catch {
		// ignore error
	}
}

async function download(
	items: EditorItem[],
	project: Project,
	id: string,
	opts?: {
		taskID?: string;
		threadID?: string;
		runID?: string;
	}
) {
	const item = items.find((item) => item.id === id);
	if (item?.file && item.file.modified && item.file.buffer) {
		await ChatService.saveContents(
			project.assistantID,
			project.id,
			item.id,
			item.file.buffer,
			opts
		);
		item.file.contents = item.file.buffer;
		item.file.modified = false;
		item.file.blob = undefined;
	}
	await ChatService.download(project.assistantID, project.assistantID, id, opts);
}

async function loadFile(
	items: EditorItem[],
	project: Project,
	file: string,
	opts?: {
		taskID?: string;
		threadID?: string;
		runID?: string;
	}
) {
	try {
		const blob = await ChatService.getFile(project.assistantID, project.id, file, opts);
		const contents = await blob.text();
		let fileID = file;
		if (opts?.taskID && opts?.runID) {
			fileID = `${opts.taskID}/${opts.runID}/${file}`;
		}
		const targetFile: EditorItem = {
			id: fileID,
			file: {
				threadID: opts?.threadID,
				buffer: '',
				modified: false,
				taskID: opts?.taskID,
				runID: opts?.runID,
				contents,
				blob
			},
			name: file,
			selected: true
		};
		for (let i = 0; i < items.length; i++) {
			if (items[i].id === targetFile.id) {
				items[i] = targetFile;
				select(items, targetFile.id);
				return;
			}
		}
		items.push(targetFile);
		select(items, targetFile.id);
	} catch {
		// ignore error
	}
}

function select(items: EditorItem[], id: string) {
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

function remove(items: EditorItem[], id: string): boolean {
	for (let i = 0; i < items.length; i++) {
		if (items[i].id === id) {
			if (i > 0) {
				select(items, items[i - 1].id);
			} else if (items.length > 1) {
				select(items, items[i + 1].id);
			}
			items.splice(i, 1);
			break;
		}
	}

	return items.length === 0;
}

export default {
	remove,
	load,
	download,
	select
};
