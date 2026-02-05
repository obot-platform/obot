import type { ChatMessageItemToolCall } from './types';

export function parseToolFilePath(item: ChatMessageItemToolCall) {
    if (!item.arguments) return null;
    const parsed = JSON.parse(item.arguments);
    const filePath = parsed.file_path;
    return filePath ? filePath.split('/').pop().split('.').shift() : null;
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
