const WINDOWS_RESERVED_FILENAME = /^(con|prn|aux|nul|com[1-9]|lpt[1-9])$/i;

export function sanitizeFilenameSegment(value: string, fallback = 'download'): string {
	const sanitized = value
		.trim()
		.toLowerCase()
		.replace(/[^a-z0-9_-]+/g, '-')
		.replace(/^-+|-+$/g, '')
		.slice(0, 200)
		.replace(/[-_]+$/g, '');
	const filename = sanitized || fallback;
	return WINDOWS_RESERVED_FILENAME.test(filename) ? `${filename}-file` : filename;
}

// saveBlob prompts a browser download of a blob under the given filename.
// Browser-only (uses document / URL); call from event handlers.
export function saveBlob(blob: Blob, filename: string): void {
	const url = URL.createObjectURL(blob);
	const a = document.createElement('a');
	a.href = url;
	a.download = filename;
	a.style.display = 'none';
	document.body.append(a);
	a.click();
	a.remove();
	setTimeout(() => URL.revokeObjectURL(url), 0);
}
