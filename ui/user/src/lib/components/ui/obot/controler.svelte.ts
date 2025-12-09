import { createContext } from 'svelte';

interface ObotUIControllerProps {
	[key: string]: any;
}

const [get, set] = createContext<ObotUIController>();

export class ObotUIController<Props extends ObotUIControllerProps = ObotUIControllerProps> {
	#props: () => Props;

	dom: Record<string, HTMLElement | null> = $state({});
	setup: Record<string, { attrs: () => Record<string, any>; fn: (node: HTMLElement) => any }> = {};

	constructor(props: () => Props) {
		this.#props = props;
	}

	get props() {
		return this.#props?.();
	}

	share(): this {
		return ObotUIController.set(this) as this;
	}

	destroy(){

	}

	static get(): unknown | undefined {
		throw new Error('Method not implemented! Use derived class');
	}

	static set(controller: unknown): unknown {
		throw new Error('Method not implemented! Use derived class');
	}
}
