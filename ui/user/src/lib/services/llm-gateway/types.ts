export interface SnippetBlock {
	/** Optional label rendered above the code block. */
	title?: string;
	language: 'bash' | 'json' | 'toml' | 'python' | 'typescript';
	code: string;
}
