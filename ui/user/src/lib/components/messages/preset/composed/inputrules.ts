import type { MilkdownPlugin } from '@milkdown/ctx';
import { inlineCodeInputRule } from '@milkdown/kit/preset/commonmark';
import { createCodeBlockInputRule, insertHrInputRule } from '@milkdown/kit/preset/commonmark';

/// @internal
export const inputRules: MilkdownPlugin[] = [createCodeBlockInputRule, insertHrInputRule].flat();

/// @internal
export const markInputRules: MilkdownPlugin[] = [inlineCodeInputRule].flat();
