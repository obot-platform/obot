import type { MilkdownPlugin } from '@milkdown/ctx';
import {
	inlineCodeAttr,
	inlineCodeSchema,
	codeBlockAttr,
	codeBlockSchema,
	docSchema,
	paragraphAttr,
	paragraphSchema,
	textSchema,
	hardbreakSchema,
	hardbreakAttr
} from '@milkdown/kit/preset/commonmark';

/// @internal
export const schema: MilkdownPlugin[] = [
	docSchema,

	paragraphAttr,
	paragraphSchema,

	hardbreakAttr,
	hardbreakSchema,

	codeBlockAttr,
	codeBlockSchema,

	inlineCodeAttr,
	inlineCodeSchema,

	textSchema
].flat();
