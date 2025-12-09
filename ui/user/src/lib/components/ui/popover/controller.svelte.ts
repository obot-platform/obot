import { ObotUIController } from '../obot/controler.svelte';
import {
	autoUpdate,
	computePosition,
	flip,
	offset,
	type ComputePositionConfig,
	type ComputePositionReturn,
	type Placement
} from '@floating-ui/dom';
import { createContext } from 'svelte';
import { createAttachmentKey } from 'svelte/attachments';

export interface PopoverControllerProps {
	open?: boolean;
	placement?: Placement;
	placements?: Placement[];
	offset?: number;
}

const [get, set] = createContext<PopoverController>();

export class PopoverController extends ObotUIController<PopoverControllerProps> {
	position = $state<ComputePositionReturn>();

	constructor(props: () => PopoverControllerProps) {
		super(props);

		this.setup = {
			trigger: {
				fn: (node) => {
					this.dom.trigger = node;
				},
				attrs: () => ({
					'aria-expanded': this.props.open ? 'true' : 'false',
					type: 'button',
					onclick: () => {
						if (this.dom.trigger instanceof HTMLButtonElement) {
							if (this.dom.trigger?.ariaDisabled === 'true' || this.dom.trigger?.disabled) return;
						}

						this.toggle();
					}
				})
			},
			content: {
				fn: (node) => {},
				attrs: () => ({
					[createAttachmentKey()]: (node: HTMLElement) => {
						// Recalculate position whenever popover is opened/closed

						this.dom.content = node;

						if (!this.dom.trigger) {
							return;
						}

						if (!this.props.open) return;

						return popover(this)(
							{
								...props,
								onchange: (node: HTMLElement, position: ComputePositionReturn) => {
									this.position = position;
								}
							},
							autoUpdate
						);
					}
				})
			}
		};
	}

	open() {
		this.props.open = true;
	}
	close() {
		this.props.open = false;
	}
	toggle() {
		this.props.open = !this.props.open;
	}

	share(): this {
		return PopoverController.set(this) as this;
	}

	static get = get;
	static set = set;
}

export type PopoverParams = {
	apply?: (
		node: HTMLElement,
		params: { x: number; y: number; dx: number; dy: number; open: boolean; offset: number }
	) => void;

	onchange?: (node: HTMLElement, params: ComputePositionReturn) => void;
};

function popover(controller: PopoverController) {
	return (props: Record<string, unknown>, updater?: typeof autoUpdate) => {
		const { offset: ofs, placements, placement } = controller.props;

		// Guard: ensure required elements exist
		if (!controller.dom.content || !controller.dom.trigger) {
			return;
		}

		const { content, trigger } = controller.dom;

		// Build middleware stack
		const middleware: ComputePositionConfig['middleware'] = [
			offset(ofs),
			flip({
				fallbackPlacements: placements,
				padding: 0
			})
		];

		// Position change callback
		const onchangeCallback = props.onchange as PopoverParams['onchange'];

		// Compute position and notify listeners
		const compute = async () => {
			// Double RAF to ensure DOM has fully settled, styles applied, and layout complete
			await new Promise((resolve) =>
				requestAnimationFrame(() => {
					requestAnimationFrame(resolve);
				})
			);

			const position = await computePosition(trigger, content, {
				placement: placement ?? 'bottom',
				middleware
			});

			onchangeCallback?.(content, position);

			// Set minimum width to match trigger after first position calculation
			content.style.minWidth = `${trigger.clientWidth}px`;
		};

		// Use auto-update if provided, otherwise compute once
		if (updater) {
			return updater(trigger, content, compute, {});
		}

		compute();
	};
}
