export * from './chat/operations';
export * from './chat/types';
export * from './admin/types';
export { default as AdminService } from './admin';
export { default as ChatService } from './chat';
export { default as EditorService } from './editor/index.svelte';
export type { Fetcher } from './http';

export async function updateMemory(
	assistantId: string,
	projectId: string,
	memoryId: string,
	content: string
) {
	const response = await fetch(
		`/api/assistants/${assistantId}/projects/${projectId}/memories/${memoryId}`,
		{
			method: 'PUT',
			headers: {
				'Content-Type': 'application/json'
			},
			body: JSON.stringify({ content })
		}
	);

	if (!response.ok) {
		throw new Error(`Failed to update memory: ${response.status} ${response.statusText}`);
	}

	return response.json();
}
