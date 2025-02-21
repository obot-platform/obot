import type { EditorItem } from '$lib/services/editor/index.svelte';
import { getContext, hasContext, setContext } from 'svelte';

export function initEditorItems(items: EditorItem[]) {
	const data = $state<EditorItem[]>(items);
	setContext('editorItems', data);
}

export function getEditorItems(): EditorItem[] {
	if (!hasContext('editorItems')) {
		throw new Error('editorItems context not initialized');
	}
	return getContext<EditorItem[]>('editorItems');
}

export function hasEditorItems(): boolean {
	return hasContext('editorItems');
}
