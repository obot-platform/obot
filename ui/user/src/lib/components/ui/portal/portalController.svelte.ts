import { ObotUIController } from '../obot/controler.svelte';
import { PortalHubController } from './hubController.svelte';
import { createContext } from 'svelte';
import { createAttachmentKey } from 'svelte/attachments';

export interface PortalControllerProps {
	id: string;
}

const [get, set] = createContext<PortalController>();

export class PortalController extends ObotUIController<PortalControllerProps> {
	#hub = PortalHubController.get();

	constructor(props: () => PortalControllerProps) {
		super(props);

		if (this.#hub) {
			this.#hub.register(this.props.id, this);
		}
	}

	get target() {
		return this.dom.inner ?? this.dom.root;
	}

	rootProps() {
		return {
			[createAttachmentKey()]: (node: HTMLElement) => {
				this.dom.root = node;
			}
		} as const;
	}

	innerProps() {
		return {
			[createAttachmentKey()]: (node: HTMLElement) => {
				this.dom.inner = node;
			}
		} as const;
	}

	destroy(): void {
		this.#hub?.unregister(this.props.id);
		super.destroy();
	}

	share(): this {
		return PortalController.set(this) as this;
	}

	static get = get;
	static set = set;
}
