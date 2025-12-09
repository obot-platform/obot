import { ObotUIController } from '../obot/controler.svelte';
import type { PortalController } from './portalController.svelte';
import { createContext } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';

export interface PortalHubControllerProps {
	id?: string;
}

const [get, set] = createContext<PortalHubController>();

export class PortalHubController extends ObotUIController<PortalHubControllerProps> {
	#portals = new SvelteMap<string, PortalController>();

	constructor(props: () => PortalHubControllerProps) {
		super(props);
	}

	register(id: string, controller: PortalController) {
		this.#portals.set(id, controller);
	}

	unregister(id: string) {
		this.#portals.delete(id);
	}

	portal(id: string): PortalController | undefined {
		return this.#portals.get(id);
	}

    share(): this {
		return PortalHubController.set(this) as this;
	}

	static get = get;
	static set = set;
}
