import { getContext, setContext } from 'svelte';

const CONTEXT_KEY = Symbol('@obot/components/virtual-page');

export type VirtualPageContext<T> = {
	elements: {
		viewport: HTMLElement | undefined;
		content: HTMLElement | undefined;
	};

	readonly height: number;

	readonly overscan: number;
	readonly itemHeight: number;
	readonly scrollToIndex: number | undefined;

	top: number;
	bottom: number;
	disabled: boolean;

	rows: {
		index: number;
		data: T;
	}[];

	data: T[];
};

export function getVirtualPageContext<T>(): VirtualPageContext<T> | undefined {
	return getContext(CONTEXT_KEY);
}

export function setVirtualPageContext<T>(context: VirtualPageContext<T>) {
	return setContext(CONTEXT_KEY, context);
}

export function setVirtualPageData<T>(data: T[]) {
	const context = getVirtualPageContext<T>();
	if (context) {
		context.data = data;
	}
}

export function setVirtualPageDisabled<T>(disabled: boolean) {
	const context = getVirtualPageContext<T>();
	if (context) {
		context.disabled = disabled;
	}
}
