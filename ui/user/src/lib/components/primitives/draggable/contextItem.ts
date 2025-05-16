import { getContext, setContext } from 'svelte';

const CONTEXT_KEY = '@obot/context/dragable-root/item';

export type DraggableItem<T = unknown> = {
	id: string;
	data: T;
};

export type DraggableItemContext<T = unknown> = {
	readonly state: {
		id: DraggableItem<T>[];
		data?: T;
		onPointerDown?: (ev: PointerEvent) => void;
		onPointerEnter?: (ev: PointerEvent) => void;
		onPointerLeave?: (ev: PointerEvent) => void;
	};
};
export function getDraggableItemContext<T = unknown>(): DraggableItemContext<T> {
	return getContext(CONTEXT_KEY);
}

export function setDraggableItemContext<T>(context: DraggableItemContext<T>) {
	return setContext(CONTEXT_KEY, context);
}
