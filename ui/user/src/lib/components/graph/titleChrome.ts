import type { Snippet } from 'svelte';

export const TITLE_CHROME = Symbol('title-chrome');

export type TitleChrome = {
	leading?: Snippet;
	trailing?: Snippet;
};
