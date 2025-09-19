import type { MilkdownPlugin } from '@milkdown/ctx';
import {
	hardbreakClearMarkPlugin,
	hardbreakFilterNodes,
	hardbreakFilterPlugin,
	inlineNodesCursorPlugin,
	remarkHtmlTransformer,
	remarkLineBreak,
	remarkMarker
} from '@milkdown/kit/preset/commonmark';

/// @internal
export const plugins: MilkdownPlugin[] = [
	hardbreakClearMarkPlugin,
	hardbreakFilterNodes,
	hardbreakFilterPlugin,

	inlineNodesCursorPlugin,

	remarkLineBreak,
	remarkHtmlTransformer,
	remarkMarker
].flat();
