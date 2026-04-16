import type { FileTimeResult } from './services/nanobot/types';
import type { TimeDisplayFormat } from './time';

export function formatNumber(num: number): string {
	if (num >= 1000) {
		const thousands = num / 1000;
		return thousands % 1 === 0 ? `${thousands}k` : `${thousands.toFixed(1)}k`;
	}
	return num.toString();
}

export function formatFileSize(bytes: number): string {
	const units = ['B', 'KB', 'MB', 'GB'];
	let size = bytes;
	let unitIndex = 0;

	while (size >= 1024 && unitIndex < units.length - 1) {
		size /= 1024;
		unitIndex++;
	}

	return `${size.toFixed(1)} ${units[unitIndex]}`;
}

export function formatFileTime(timestamp: unknown, format: TimeDisplayFormat): FileTimeResult {
	if (typeof timestamp !== 'string') return { date: undefined, formatted: '' };

	const value = timestamp.trim();
	if (!value) return { date: undefined, formatted: '' };

	const date = new Date(value);
	if (Number.isNaN(date.getTime())) return { date: undefined, formatted: '' };

	let formatted = '';
	try {
		formatted = new Intl.DateTimeFormat(undefined, {
			year: 'numeric',
			month: 'numeric',
			day: 'numeric',
			hour: '2-digit',
			minute: '2-digit',
			hour12: format === '12h'
		}).format(date);
	} catch {
		return { date: undefined, formatted: '' };
	}

	return { date, formatted };
}

function convertBase64ToBytes(base64: string) {
	const binary = atob(base64);
	const bytes = new Uint8Array(binary.length);
	for (let i = 0; i < binary.length; i++) {
		bytes[i] = binary.charCodeAt(i);
	}
	return bytes;
}

export const formatBase64ToBlobUrl = (base64: string, mime: string): string => {
	const bytes = convertBase64ToBytes(base64);
	return URL.createObjectURL(new Blob([bytes], { type: mime }));
};

export const formatBase64ToBlob = (base64: string, type: string): Blob => {
	const bytes = convertBase64ToBytes(base64);
	return new Blob([bytes], { type });
};
