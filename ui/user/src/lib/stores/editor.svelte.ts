import type { Task } from '$lib/services/chat/types';

export interface EditorItem {
	id: string;
	name: string;
	contents: string;
	blob?: Blob;
	buffer: string;
	modified?: boolean;
	selected?: boolean;
	task?: Task;
	table?: string;
	taskID?: string;
	runID?: string;
}

const items = $state<EditorItem[]>([]);

export default items;
