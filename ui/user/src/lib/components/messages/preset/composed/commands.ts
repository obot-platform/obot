import type { MilkdownPlugin } from '@milkdown/ctx';
import {
	addBlockTypeCommand,
	clearTextInCurrentBlockCommand,
	insertHardbreakCommand,
	isMarkSelectedCommand,
	isNodeSelectedCommand,
	selectTextNearPosCommand,
	setBlockTypeCommand,
	wrapInBlockTypeCommand
} from '@milkdown/kit/preset/commonmark';
import { toggleInlineCodeCommand } from '@milkdown/kit/preset/commonmark';
import {
	createCodeBlockCommand,
	insertHrCommand,
	turnIntoTextCommand
} from '@milkdown/kit/preset/commonmark';

/// @internal
export const commands: MilkdownPlugin[] = [
	turnIntoTextCommand,
	createCodeBlockCommand,
	insertHardbreakCommand,
	insertHrCommand,
	toggleInlineCodeCommand,

	isMarkSelectedCommand,
	isNodeSelectedCommand,

	clearTextInCurrentBlockCommand,
	setBlockTypeCommand,
	wrapInBlockTypeCommand,
	addBlockTypeCommand,
	selectTextNearPosCommand
];
