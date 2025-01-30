import type { Task } from '$lib/services/chat/types';

export interface EditorItem {
	id: string;
	name: string;
	contents: string;
	blob?: Blob;
	buffer: string;
	modified?: boolean;
	selected?: boolean;
	generic?: boolean;
	task?: Task;
	table?: string;
	taskID?: string;
	runID?: string;
}

const store = $state({
	items: [] as EditorItem[]
});

export default store;
