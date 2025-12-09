import { createContext } from 'svelte';

interface ObotUIControllerProps {
	[key: string]: any;
}

const [get, set] = createContext<ObotUIController>();

export class ObotUIController<Props extends ObotUIControllerProps = ObotUIControllerProps> {
	#id: string = generateId();
	#props: () => Props;

	dom: Record<string, HTMLElement | null> = $state({});

	constructor(props: () => Props) {
		this.#props = props;
	}

	get id() {
		return this.#id;
	}

	get props() {
		return this.#props?.();
	}

	share(): this {
		return ObotUIController.set(this) as this;
	}

	destroy() {}

	static get(): unknown | undefined {
		throw new Error('Method not implemented! Use derived class');
	}

	static set(controller: unknown): unknown {
		throw new Error('Method not implemented! Use derived class');
	}
}

function generateId(prefix = 'obot'): string {
	return `${prefix}-${(Date.now() + Math.random() * Math.random()).toString(36).substring(2, 11)}`;
}
