import { goto } from '$app/navigation';
import { Role } from './services';

// Simple delay function
export function delay(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}

// Simple throttle function
export function throttle<T extends (...args: Parameters<T>) => ReturnType<T>>(
	func: T,
	delay: number
): T {
	let timeoutId: number | null = null;
	return ((...args: Parameters<T>) => {
		if (timeoutId) return;
		timeoutId = setTimeout(() => {
			func(...args);
			timeoutId = null;
		}, delay);
	}) as T;
}

// Poll a function until it returns true, or until a timeout is reached.
// Returns when the function returns true.
// Throws an exception if the timeout is reached before the function returns true.
export async function poll(
	pollFn: () => Promise<boolean>,
	options: {
		interval?: number;
		maxTimeout?: number;
	} = {}
): Promise<void> {
	const { interval = 1000, maxTimeout = 30000 } = options;
	const startTime = Date.now();

	while (true) {
		if (await pollFn()) {
			return;
		}

		if (Date.now() - startTime >= maxTimeout) {
			throw new Error(`Poll timeout after ${maxTimeout}ms`);
		}

		await delay(interval);
	}
}

// File type detection utilities
export const TEXT_FILE_EXTENSIONS = {
	markup: ['md', 'txt', 'rst', 'adoc', 'asciidoc', 'tex', 'bib'],
	code: [
		'js',
		'ts',
		'jsx',
		'tsx',
		'py',
		'java',
		'c',
		'cpp',
		'h',
		'hpp',
		'cs',
		'php',
		'rb',
		'go',
		'rs',
		'swift',
		'kt',
		'scala',
		'r',
		'm',
		'pl',
		'sh',
		'bash',
		'zsh',
		'fish',
		'ps1',
		'bat',
		'cmd',
		'psm1',
		'psd1'
	],
	web: [
		'html',
		'htm',
		'css',
		'scss',
		'sass',
		'less',
		'xml',
		'svg',
		'vue',
		'svelte',
		'astro',
		'jsx',
		'tsx'
	],
	// Data formats
	data: [
		'json',
		'yaml',
		'yml',
		'toml',
		'ini',
		'cfg',
		'conf',
		'config',
		'env',
		'csv',
		'tsv',
		'sql',
		'graphql',
		'gql',
		'rss',
		'atom'
	],
	config: [
		'makefile',
		'dockerfile',
		'dockerignore',
		'gitignore',
		'gitattributes',
		'editorconfig',
		'babelrc',
		'eslintrc',
		'prettierrc',
		'browserslist',
		'npmrc',
		'yarnrc'
	],
	docs: [
		'readme',
		'license',
		'changelog',
		'version',
		'contributing',
		'code_of_conduct',
		'security',
		'support',
		'faq',
		'troubleshooting'
	],
	scripts: [
		'sh',
		'bash',
		'zsh',
		'fish',
		'ps1',
		'bat',
		'cmd',
		'psm1',
		'psd1',
		'py',
		'rb',
		'pl',
		'lua',
		'tcl',
		'awk',
		'sed'
	]
};

/**
 * Check if a file is a text file based on its extension
 */
export function isTextFile(filename: string): boolean {
	if (!filename) return false;

	const extension = filename.toLowerCase().split('.').pop();
	if (!extension) return false;

	// Check all text file categories
	return Object.values(TEXT_FILE_EXTENSIONS).some((category) => category.includes(extension));
}

export function openUrl(url: string, isCtrlClick: boolean) {
	if (isCtrlClick) {
		window.open(url, '_blank');
	} else {
		goto(url);
	}
}

export const getUserRoleLabel = (role: number) => {
	if (role === Role.ADMIN) return 'Admin';
	if (role === Role.POWERUSER) return 'Power User';
	if (role === Role.POWERUSER_PLUS) return 'Power User Plus';
	if (role === Role.USER) return 'User';
	return 'Unknown';
};
