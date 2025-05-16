import { getContext, setContext } from 'svelte';

const CONTEXT_KEY = '@obot/context/dragable-root';

export type DraggableItem<T = unknown> = {
	id: string;
	data: T;
};

export type DraggableContext<T = unknown> = {
	readonly state: {
		items: DraggableItem<T>[];
		sourceItemId?: string;
		targetItemId?: string;
		disabled?: boolean;
	};
	methods: {
		reorder: () => void;
		mount: (id: string, item: { id: string; data: T }) => () => void;
		unmount: (id: string) => void;
		setSourceItem: (id?: string) => void;
		setTargetItem: (id?: string) => void;
	};
};
export function getDraggableContext<T = unknown>(): DraggableContext<T> {
	return getContext(CONTEXT_KEY);
}

export function setDraggableContext<T>(context: DraggableContext<T>) {
	return setContext(CONTEXT_KEY, context);
}
