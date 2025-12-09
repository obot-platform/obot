import { createAttachmentKey } from 'svelte/attachments';
import { ObotUIController } from '../obot/controler.svelte';
import { PortalHubController } from './hubController.svelte';
import { createContext } from 'svelte';

export interface PortalControllerProps {
	id: string;
}

const [get, set] = createContext<PortalController>();

export class PortalController extends ObotUIController<PortalControllerProps> {
	#hub = PortalHubController.get();

	constructor(props: () => PortalControllerProps) {
		super(props);

		this.setup = {
			root: {
				fn: (node) => {
				},
				attrs: () => ({
					[createAttachmentKey()]: (node: HTMLElement) => {
						this.dom.root = node;
					}
				})
			},
			inner: {
				fn: (node) => {
				},
				attrs: () => ({
					[createAttachmentKey()]: (node: HTMLElement) => {
						this.dom.inner = node;
					}
				})
			}
		};

		if (this.#hub) {
			this.#hub.register(this.props.id, this);
		}
	}

	get target() {
		return this.dom.inner ?? this.dom.root;
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
