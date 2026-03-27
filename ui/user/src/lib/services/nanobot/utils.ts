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

export function normalizeResourceReadURI(uri: string): string {
	if (!uri) return uri;

	const fileURIPrefix = 'file:///';
	const homePrefix = '/home/nanobot/';
	const isFileURI = uri.startsWith(fileURIPrefix);
	const isWorkspacePath = uri.startsWith(homePrefix);

	if (!isFileURI && !isWorkspacePath) {
		return uri;
	}

	const rawPath = isFileURI ? uri.slice(fileURIPrefix.length) : uri.slice(homePrefix.length);
	const normalizedPath = rawPath
		.split('/')
		.map((segment) => {
			try {
				return encodeURIComponent(decodeURIComponent(segment));
			} catch {
				return encodeURIComponent(segment);
			}
		})
		.join('/');

	return `${fileURIPrefix}${normalizedPath}`;
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

export function parseJSON<T>(json?: string): T | null {
	if (!json) return null;
	try {
		return JSON.parse(json) as T;
	} catch {
		return null;
	}
}

export function isPolicyViolation(content: string | undefined): boolean {
	if (!content) return false;
	return content.includes('<system_notification>');
}

export function extractPolicyExplanation(content: string): string {
	const match = content.match(/<system_notification>([\s\S]*?)<\/system_notification>/);
	if (!match) return content;
	const inner = match[1].trim();
	const explanationMatch = inner.match(
		/(?:for the user|relay to the user):\s*([\s\S]*)/i
	);
	return explanationMatch ? explanationMatch[1].trim() : inner;
}

export function splitFrontmatter(markdown: string): { frontmatter: string; body: string } {
	const trimmed = markdown.trimStart();
	if (!trimmed.startsWith('---')) {
		return { frontmatter: '', body: markdown };
	}
	const afterFirstFence = trimmed.slice(3);
	const secondFenceIndex = afterFirstFence.indexOf('\n---');
	if (secondFenceIndex === -1) {
		return { frontmatter: '', body: markdown };
	}
	const fenceEnd = afterFirstFence.indexOf('\n---') + 4; // include \n---
	const frontmatter = markdown.slice(0, markdown.length - trimmed.length + 3 + fenceEnd);
	const body = markdown.slice(markdown.length - trimmed.length + 3 + fenceEnd).replace(/^\n?/, '');
	return { frontmatter, body };
}

export type FrontmatterParsed = {
	metadata?: Record<string, unknown>;
	[name: string]: unknown;
};

export function parseFrontmatter(frontmatterYaml: string): FrontmatterParsed {
	if (!frontmatterYaml.trim()) return {};
	try {
		const out: FrontmatterParsed = {};
		const lines = frontmatterYaml.split(/\r?\n/);
		let inMetadata = false;
		let metadataIndent = 0;
		for (const line of lines) {
			const trimmed = line.trimEnd();
			if (!trimmed) continue;
			const indent = line.length - line.trimStart().length;
			if (inMetadata && indent <= metadataIndent && trimmed) {
				inMetadata = false;
			}
			const keyMatch = trimmed.match(/^\s*(\w[-\w]*)\s*:\s*(.*)$/);
			if (!keyMatch) continue;
			const [, key, value] = keyMatch;
			const rawValue = value.trim().replace(/^["']|["']$/g, '');
			if (key === 'metadata') {
				inMetadata = true;
				metadataIndent = indent;
				out.metadata = out.metadata ?? {};
				continue;
			}
			if (inMetadata && indent > metadataIndent) {
				out.metadata = out.metadata ?? {};
				(out.metadata as Record<string, unknown>)[key] = rawValue || value.trim();
				continue;
			}
			if (!inMetadata) (out as Record<string, unknown>)[key] = rawValue || value.trim();
		}
		return out;
	} catch {
		return {};
	}
}
