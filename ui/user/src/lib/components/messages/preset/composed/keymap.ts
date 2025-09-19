import type { MilkdownPlugin } from '@milkdown/ctx';
import { inlineCodeKeymap } from '@milkdown/kit/preset/commonmark';
import { codeBlockKeymap, hardbreakKeymap, paragraphKeymap } from '@milkdown/kit/preset/commonmark';

export const keymap: MilkdownPlugin[] = [
	codeBlockKeymap,
	hardbreakKeymap,
	paragraphKeymap,
	inlineCodeKeymap
].flat();
