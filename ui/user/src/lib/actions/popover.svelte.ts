import {
	type ComputePositionConfig,
	type Placement,
	autoUpdate,
	computePosition,
	flip,
	offset,
	shift
} from '@floating-ui/dom';
import { tick } from 'svelte';
import type { Action } from 'svelte/action';

interface TooltipOptions {
	slide?: 'left' | 'up';
	fixed?: boolean;
}

interface Popover {
	ref: Action<HTMLElement>;
	tooltip: Action<HTMLElement, TooltipOptions | undefined>;
	open: boolean;
	toggle: (newOpenValue?: boolean) => void;
}

interface PopoverOptions extends Partial<ComputePositionConfig> {
	hover?: boolean;
	assign?: (x: number, y: number) => void;
	offset?: number;
	placement?: Placement;
	delay?: number;
	onOpenChange?: (open: boolean) => void;
}

let id = 0;

export default function popover(initialOptions?: PopoverOptions): Popover {
	let ref: HTMLElement;
	let tooltip: HTMLElement;
	let hoverTimeout: number | null = null;

	let open = $state(false);
	const options = $state<PopoverOptions & TooltipOptions>(initialOptions ?? {});
	const selfId = $state(id++);

	$effect(() => {
		if (!ref || !tooltip) return;

		const handler = (e: Event) => {
			if (e instanceof CustomEvent && e.detail !== selfId.toString()) {
				open = false;
				options?.onOpenChange?.(open);
			}
		};
		document.addEventListener('toolOpen', handler);
		return () => document.removeEventListener('toolOpen', handler);
	});

	$effect(() => {
		if (!ref || !tooltip) return;
		if (!open || options?.hover) return;

		document.querySelector('#click-catch')?.remove();
		const div = document.createElement('div');
		div.id = 'click-catch';
		div.classList.add('fixed', 'inset-0', 'z-10', 'cursor-default');
		div.onclick = () => {
			open = false;
			div.remove();
			options?.onOpenChange?.(open);
		};

		ref.insertAdjacentElement('afterend', div);
		return () => div.remove();
	});

	$effect(() => {
		if (!ref || !tooltip || !options?.hover) return;

		const onEnter = () => {
			if (hoverTimeout) return;
			hoverTimeout = setTimeout(() => {
				hoverTimeout = null;
				if (!open) {
					open = true;
					options.onOpenChange?.(open);
				}
			}, options.delay ?? 150);
		};

		const onLeave = () => {
			if (hoverTimeout) {
				clearTimeout(hoverTimeout);
				hoverTimeout = null;
			}
			if (open) {
				open = false;
				options.onOpenChange?.(open);
			}
		};

		ref.addEventListener('mouseenter', onEnter);
		ref.addEventListener('mouseleave', onLeave);
		return () => {
			ref.removeEventListener('mouseenter', onEnter);
			ref.removeEventListener('mouseleave', onLeave);
		};
	});

	let close: (() => void) | null;
	$effect(() => {
		if (!ref || !tooltip) return;

		// Remove all dynamically added classes for proper reset
		tooltip.classList.remove(
			'fixed',
			'absolute',
			'translate-x-full',
			'translate-y-full',
			'translate-x-0',
			'translate-y-0',
			'transition-opacity',
			'transition-[transform,opacity]',
			'transform',
			'hidden',
			'opacity-0',
			'duration-300'
		);

		// Reset positioning styles
		tooltip.style.removeProperty('left');
		tooltip.style.removeProperty('top');

		tooltip.classList.add(options?.fixed ? 'fixed' : 'absolute');

		if (options?.slide) {
			tooltip.classList.add(
				'transition-[transform,opacity]',
				'transform',
				'duration-300',
				options.slide === 'left' ? 'translate-x-full' : 'translate-y-full',
				'opacity-0'
			);
		} else {
			tooltip.classList.add('hidden', 'transition-opacity', 'duration-300', 'opacity-0');
		}

		// Handle z-index
		let hasZIndex = false;
		tooltip.classList.forEach((className) => {
			if (className.startsWith('z-')) hasZIndex = true;
		});
		if (!hasZIndex) tooltip.classList.add('z-40');

		// Handle visibility and positioning
		if (open) {
			tick().then(() => {
				if (options?.slide) {
					tooltip.classList.remove(
						options.slide === 'left' ? 'translate-x-full' : 'translate-y-full'
					);
					tooltip.classList.add(options.slide === 'left' ? 'translate-x-0' : 'translate-y-0');
				} else {
					tooltip.classList.remove('hidden');
				}
				tooltip.classList.remove('opacity-0');

				if (!options?.fixed) {
					updatePosition().then(() => {
						close = autoUpdate(ref, tooltip, updatePosition);
					});
				}
			});
		} else {
			close?.();
			if (options?.slide) {
				tooltip.classList.add(options.slide === 'left' ? 'translate-x-full' : 'translate-y-full');
			} else {
				tooltip.classList.add('hidden');
			}
			tooltip.classList.add('opacity-0');
			close = null;
		}
	});

	async function updatePosition() {
		if (!ref || !tooltip || options?.fixed) return;

		const offsetSize = options?.offset ?? 4;
		const { x, y } = await computePosition(ref, tooltip, {
			placement: options?.placement ?? 'bottom-end',
			middleware: [flip(), shift({ padding: offsetSize }), offset(offsetSize)],
			...options
		});

		if (options?.assign) {
			options.assign(x, y);
		} else {
			Object.assign(tooltip.style, {
				left: `${x}px`,
				top: `${y}px`
			});
		}
	}

	return {
		get open() {
			return open;
		},
		ref: (node: HTMLElement) => {
			ref = node;
		},
		tooltip: (node: HTMLElement, params?: TooltipOptions) => {
			tooltip = node;
			if (params) {
				Object.assign(options, params);
			}

			return {
				update(newParams?: TooltipOptions) {
					if (newParams) {
						Object.assign(options, newParams);
					}
				}
			};
		},
		toggle: (newOpenValue?: boolean) => {
			if (!open && !options?.hover) {
				document.dispatchEvent(new CustomEvent('toolOpen', { detail: selfId.toString() }));
			}
			open = newOpenValue ?? !open;
			options?.onOpenChange?.(open);
		}
	};
}
