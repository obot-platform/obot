import type { FileTimeResult } from './services/nanobot/types';
import type { TimeDisplayFormat } from './time';

function formatWithSuffix(value: number, suffix: string): string {
	return value % 1 === 0 ? `${value}${suffix}` : `${value.toFixed(1)}${suffix}`;
}

export function formatNumber(num: number): string {
	if (num < 1000) {
		return num.toString();
	}
	if (num >= 1_000_000_000) {
		return formatWithSuffix(num / 1_000_000_000, 'B');
	}
	if (num >= 1_000_000) {
		return formatWithSuffix(num / 1_000_000, 'M');
	}
	return formatWithSuffix(num / 1000, 'k');
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

	let formatted: string;
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

export const formatDeviceCommand = (cmd?: string, args?: string[]): string => {
	if (!cmd) return '—';
	const parts = [cmd, ...(args ?? [])];
	return parts.join(' ');
};

const AGENTS_HOME_PATTERN = /^(?:\/(?:Users|home|root)\/[^/]+|~)\/\.agents(?:\/|$)/;

export const isAgentsHomeProjectPath = (projectPath?: string): boolean => {
	if (!projectPath) return false;
	return AGENTS_HOME_PATTERN.test(projectPath);
};

export const deriveDeviceScope = (projectPath?: string): string => {
	if (!projectPath) return 'global';
	if (isAgentsHomeProjectPath(projectPath)) return 'global';
	return 'project';
};

export const MULTI_CLIENT_NAME = 'multi';
export const AGENTS_HOME_CLIENT_LABEL = '~/.agents';

export const formatDeviceClient = (client?: string, projectPath?: string): string => {
	if (!client) return '—';
	if (client.trim() === MULTI_CLIENT_NAME && isAgentsHomeProjectPath(projectPath)) {
		return AGENTS_HOME_CLIENT_LABEL;
	}
	return client;
};

export const formatDeviceClients = (
	clients?: string[],
	client?: string,
	projectPath?: string
): string => {
	const values = clients?.length ? clients : client ? [client] : [];
	const formatted = values.map((value) => formatDeviceClient(value, projectPath));
	return Array.from(new Set(formatted)).join(', ') || '—';
};
