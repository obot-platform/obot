import type { ChatMessageItemToolCall } from './types';

export const CANCELLATION_PHRASE_CLIENT =
	'REQUEST CANCELLED BY CLIENT: USER REQUESTED CANCELLATION';

export function isCancellationError(text: string | undefined): boolean {
	if (!text) return false;
	return text.includes('User requested cancellation') || text.includes(CANCELLATION_PHRASE_CLIENT);
}

export function parseToolFilePath(item: ChatMessageItemToolCall) {
	if (!item.arguments) return null;
	return parseJSON<{ file_path: string }>(item.arguments)?.file_path ?? null;
}

const SAFE_IMAGE_MIME_TYPES = new Set<string>([
	'image/png',
	'image/jpeg',
	'image/jpg',
	'image/webp',
	'image/gif'
]);

export function isSafeImageMimeType(mimeType: string | null | undefined): boolean {
	return !!mimeType && SAFE_IMAGE_MIME_TYPES.has(mimeType);
}

export function parseJSON<T>(json?: string): T | null {
	if (!json) return null;
	try {
		return JSON.parse(json) as T;
	} catch {
		return null;
	}
}
