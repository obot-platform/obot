import type { Fetcher, Project, ProjectTemplate } from '$lib/services';
import ChatService from '../chat';
import { doPost } from '../chat/http';

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

	selected?: boolean;
	generic?: boolean;
}

export interface GenerateImageRequest {
	prompt: string;
}

export interface ImageResponse {
	imageUrl: string;
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
	} else {
		await loadFile(items, project, id, opts);
	}
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

async function save(
	item: EditorItem,
	project: Project | ProjectTemplate,
	opts?: {
		taskID?: string;
		threadID?: string;
		runID?: string;
	}
) {
	if (!item.file || !item.file?.modified) {
		return;
	}

	await ChatService.saveContents(
		project.assistantID,
		project.id,
		item.name,
		item.file.buffer,
		opts
	);
}

async function download(
	items: EditorItem[],
	project: Project | ProjectTemplate,
	name: string,
	opts?: {
		taskID?: string;
		threadID?: string;
		runID?: string;
	}
) {
	const item = items.find((item) => item.name === name);
	if (item?.file && item.file.modified && item.file.buffer) {
		await save(item, project, opts);
	}
	await ChatService.download(project.assistantID, project.id, name, opts);
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
		} else if (opts?.threadID) {
			fileID = `${opts.threadID}/${file}`;
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

async function generateImage(prompt: string): Promise<ImageResponse> {
	return (await doPost('/image/generate', { prompt }, { dontLogErrors: true })) as ImageResponse;
}

async function uploadImage(file: File): Promise<ImageResponse> {
	const formData = new FormData();
	formData.append('image', file);

	return (await doPost('/image/upload', formData)) as ImageResponse;
}

async function createObot(opts?: { fetch?: Fetcher }) {
	const assistants = (await ChatService.listAssistants(opts)).items;
	let defaultAssistant = assistants.find((a) => a.default);
	if (!defaultAssistant && assistants.length == 1) {
		defaultAssistant = assistants[0];
	}
	if (!defaultAssistant) {
		throw new Error('failed to find default assistant');
	}

	return await ChatService.createProject(defaultAssistant.id, opts);
}

export default {
	remove,
	load,
	download,
	select,
	generateImage,
	uploadImage,
	createObot,
	save
};
