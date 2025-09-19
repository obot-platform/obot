import { commands, inputRules, keymap, markInputRules, plugins, schema } from './composed';
import type { MilkdownPlugin } from '@milkdown/ctx';

export const plaintext: MilkdownPlugin[] = [
	schema,
	inputRules,
	markInputRules,
	commands,
	keymap,
	plugins
].flat();
