export function isImage(filename: string): boolean {
	return /\.(jpe?g|png|gif|bmp|webp)$/i.test(filename);
}

const SAFE_IMAGE_MIME_TYPES = new Set<string>([
	'image/png',
	'image/jpeg',
	'image/jpg',
	'image/webp',
	'image/gif',
	'image/svg+xml'
]);

export function isSafeImageMimeType(mimeType: string | null | undefined): boolean {
	return !!mimeType && SAFE_IMAGE_MIME_TYPES.has(mimeType);
}
