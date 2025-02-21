import {
	type ComputePositionConfig,
	autoUpdate,
	computePosition,
	flip,
	offset,
	shift
} from '@floating-ui/dom';
import { tick } from 'svelte';
import type { Action, ActionReturn } from 'svelte/action';

interface Popover {
	ref: Action;
	tooltip: Action;
	toggle: (newOpenValue?: boolean) => void;
}

interface PopoverOptions extends Partial<ComputePositionConfig> {
	hover?: boolean;
	assign?: (x: number, y: number) => void;
	offset?: number;
}

let id = 0;

export default function popover(opts?: PopoverOptions): Popover {
	let ref: HTMLElement;
	let tooltip: HTMLElement;
	let open = $state(false);
	const offsetSize = opts?.offset ?? 8;

	function build(): ActionReturn | void {
		if (!ref || !tooltip) return;

		const selfId = id++;
		document.addEventListener('toolOpen', (e: Event) => {
			if (e instanceof CustomEvent && e.detail !== selfId.toString()) {
				open = false;
			}
		});

		async function updatePosition() {
			const { x, y } = await computePosition(ref, tooltip, {
				placement: 'bottom-end',
				middleware: [flip(), shift({ padding: offsetSize }), offset(offsetSize)],
				...opts
			});

			if (opts?.assign) {
				opts.assign(x, y);
			} else {
				Object.assign(tooltip.style, {
					left: `${x}px`,
					top: `${y}px`
				});
			}
		}

		$effect(() => {
			if (!open) {
				return;
			}

			if (!opts?.hover) {
				const div = document.createElement('div');
				div.classList.add('fixed', 'inset-0', 'z-10', 'cursor-default');
				div.onclick = () => {
					open = false;
					div.remove();
				};

				document.body.append(div);

				return () => {
					if (!open) div.remove();
				};
			}
		});

		tooltip.classList.add('hidden');
		tooltip.classList.add('absolute');
		tooltip.classList.add('transition-opacity');
		tooltip.classList.add('duration-300');
		tooltip.classList.add('opacity-0');

		let hasZIndex = false;
		tooltip.classList.forEach((className) => {
			if (className.startsWith('z-')) {
				hasZIndex = true;
			}
		});
		if (!hasZIndex) {
			tooltip.classList.add('z-30');
		}

		if (opts?.hover) {
			ref.addEventListener('mouseenter', () => {
				open = true;
			});
			ref.addEventListener('mouseleave', () => {
				open = false;
			});
		}

		let close: (() => void) | null;
		$effect(() => {
			if (open) {
				tooltip.classList.remove('hidden');
				tick().then(() => {
					tooltip.classList.remove('opacity-0');
				});
				updatePosition();
				close = autoUpdate(ref, tooltip, updatePosition);
			} else {
				close?.();
				tooltip.classList.add('hidden');
				tooltip.classList.add('opacity-0');
				close = null;
			}
		});

		return {
			destroy() {
				close?.();
			}
		};
	}

	return {
		ref: (node: HTMLElement) => {
			ref = node;
			return build();
		},
		tooltip: (node: HTMLElement) => {
			tooltip = node;
			return build();
		},
		toggle: (newOpenValue?: boolean) => {
			if (!open && !opts?.hover) {
				document.dispatchEvent(new CustomEvent('toolOpen', { detail: id.toString() }));
			}

			open = newOpenValue ?? !open;
		}
	};
}
