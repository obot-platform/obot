import { listProjects, createProject, getProject } from '$lib/services/chat/operations';
import type { Project } from '$lib/services/chat/types';
import errors from './errors.svelte';
import threads from './threads.svelte';

export interface Context {
	assistantID: string;
	projectID: string;
	project?: Project;
	valid?: boolean;
	editMode?: boolean;
	sidebarOpen?: boolean;
	currentThreadID?: string;
}

type ContextCallback = (context: Context) => Promise<void> | void;

const cbs: ContextCallback[] = [];

const store = $state<Context>({
	assistantID: '',
	projectID: 'default',
	valid: false
});

export function onInit(cb: ContextCallback) {
	if (store.valid) {
		start(cb);
		return;
	}
	cbs.push(cb);
}

function start(cb: ContextCallback) {
	if (typeof window === 'undefined') {
		return;
	}

	setTimeout(() => {
		cb(store);
	});
}

export async function init(
	assistantID: string,
	opts?: {
		projectID?: string;
		threadID?: string;
	}
) {
	try {
		store.assistantID = assistantID;
		if (opts?.projectID) {
			const project = await getProject(opts.projectID);
			store.projectID = opts.projectID;
			store.project = project;
		} else {
			const projects = await listProjects({ assistantID });
			if (projects.items.length === 0) {
				const project = await createProject({ name: '', default: true });
				projects.items.push(project);
			}
			store.projectID = projects.items[0].id;
			store.project = projects.items[0];
		}
		store.valid = true;
		store.currentThreadID = (await threads.createOrGetDefault()).id;
		runInit();
	} catch (e) {
		errors.append(e);
	}
}

function runInit() {
	if (store.valid) {
		store.valid = true;
		cbs.forEach(start);
		cbs.length = 0;
	}
}

export default store;
