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
	hover?: boolean;
	interactiveHover?: boolean;
	disablePortal?: boolean;
	el?: Element;
	enterTransition?: 'daisy';
}

interface Popover {
	ref: Action<HTMLElement>;
	tooltip: Action<HTMLElement, TooltipOptions | undefined>;
	open: boolean;
	toggle: (newOpenValue?: boolean) => void;
}

interface PopoverOptions extends Partial<ComputePositionConfig> {
	assign?: (x: number, y: number) => void;
	offset?: number;
	placement?: Placement;
	delay?: number;
	onOpenChange?: (open: boolean) => void;
}

let id = 0;

function tooltipOverlayUnchanged(
	prev: PopoverOptions & TooltipOptions,
	merged: PopoverOptions & TooltipOptions
): boolean {
	return (
		prev.slide === merged.slide &&
		prev.fixed === merged.fixed &&
		prev.hover === merged.hover &&
		prev.interactiveHover === merged.interactiveHover &&
		prev.disablePortal === merged.disablePortal &&
		prev.enterTransition === merged.enterTransition &&
		prev.strategy === merged.strategy &&
		prev.placement === merged.placement &&
		prev.offset === merged.offset
	);
}

export default function popover(initialOptions?: PopoverOptions): Popover {
	let ref: HTMLElement;
	let tooltip: HTMLElement;
	let hoverTimeout: ReturnType<typeof setTimeout> | null = null;

	let open = $state(false);
	let options = $state<PopoverOptions & TooltipOptions>(initialOptions ?? {});
	const selfId = $state(id++);

	// Create a new state to track when both elements are ready
	let ready = $state(false);

	// Function to check if both elements are ready
	function checkReady() {
		ready = !!(ref && tooltip);
	}

	$effect(() => {
		if (!ready) return;

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
		if (!ready) return;
		if (!open || options?.hover) return;

		document.querySelector('#click-catch')?.remove();
		const div = document.createElement('div');
		div.id = 'click-catch';
		div.classList.add('fixed', 'inset-0', 'z-30', 'cursor-default');
		div.onclick = () => {
			open = false;
			div.remove();
			options?.onOpenChange?.(open);
		};

		if (options?.el && options.disablePortal) {
			options.el.appendChild(div);
		} else if (options?.disablePortal) {
			ref.insertAdjacentElement('afterend', div);
		} else {
			document.body.appendChild(div);
		}
		return () => div.remove();
	});

	$effect(() => {
		if (!ready || !options?.hover) return;

		const interactive = options.interactiveHover ?? false;
		const closeDelayMs = 280;
		let closeTimer: ReturnType<typeof setTimeout> | null = null;

		const clearCloseTimer = () => {
			if (closeTimer != null) {
				clearTimeout(closeTimer);
				closeTimer = null;
			}
		};

		const onRefEnter = () => {
			clearCloseTimer();
			if (hoverTimeout) return;
			hoverTimeout = setTimeout(() => {
				hoverTimeout = null;
				if (!open) {
					open = true;
					options.onOpenChange?.(open);
				}
			}, options.delay ?? 150);
		};

		const onRefLeave = () => {
			if (hoverTimeout) {
				clearTimeout(hoverTimeout);
				hoverTimeout = null;
			}
			if (!open) return;
			if (interactive) {
				clearCloseTimer();
				closeTimer = setTimeout(() => {
					closeTimer = null;
					open = false;
					options.onOpenChange?.(open);
				}, closeDelayMs);
			} else {
				open = false;
				options.onOpenChange?.(open);
			}
		};

		const onTooltipEnter = () => {
			clearCloseTimer();
		};

		const onTooltipLeave = () => {
			if (open) {
				open = false;
				options.onOpenChange?.(open);
			}
		};

		ref.addEventListener('mouseenter', onRefEnter);
		ref.addEventListener('mouseleave', onRefLeave);

		const tooltipCleanups: (() => void)[] = [];
		if (interactive) {
			tooltip.addEventListener('mouseenter', onTooltipEnter);
			tooltip.addEventListener('mouseleave', onTooltipLeave);
			tooltipCleanups.push(() => tooltip.removeEventListener('mouseenter', onTooltipEnter));
			tooltipCleanups.push(() => tooltip.removeEventListener('mouseleave', onTooltipLeave));
		}

		return () => {
			clearCloseTimer();
			ref.removeEventListener('mouseenter', onRefEnter);
			ref.removeEventListener('mouseleave', onRefLeave);
			for (const u of tooltipCleanups) u();
		};
	});

	let close: (() => void) | null;
	let visibilityLayoutGen = 0;
	$effect(() => {
		if (!ready) return;

		const gen = ++visibilityLayoutGen;

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
			'duration-300',
			'tooltip-portal-daisy-host--inactive'
		);

		// Reset positioning styles
		tooltip.style.removeProperty('left');
		tooltip.style.removeProperty('top');

		const useFixedLayer = options?.strategy === 'fixed' || options?.fixed;
		tooltip.classList.add(useFixedLayer ? 'fixed' : 'absolute');
		// Always move tooltip to document.body unless disablePortal is enabled
		if (tooltip.parentElement !== document.body && !options?.disablePortal) {
			document.body.appendChild(tooltip);
		} else if (options?.disablePortal && options?.el) {
			options.el.appendChild(tooltip);
		}

		const motion = options?.slide
			? 'slide'
			: options?.enterTransition === 'daisy'
				? 'daisy'
				: 'fade';

		if (motion === 'slide') {
			tooltip.classList.add(
				'transition-all',
				'duration-300',
				options.slide === 'left' ? 'translate-x-full' : 'translate-y-full',
				'opacity-0'
			);
		} else if (motion === 'daisy') {
			tooltip.classList.add('opacity-0', 'tooltip-portal-daisy-host--inactive');
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
			tick().then(async () => {
				if (gen !== visibilityLayoutGen) return;
				if (motion === 'daisy') {
					if (!shouldSkipAutoPosition()) {
						await updatePosition();
					}
					if (gen !== visibilityLayoutGen) return;
					await tick();
					if (gen !== visibilityLayoutGen) return;
					await new Promise<void>((resolve) =>
						requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
					);
					if (gen !== visibilityLayoutGen) return;
					tooltip.classList.remove('tooltip-portal-daisy-host--inactive', 'opacity-0');
					if (!shouldSkipAutoPosition()) {
						close = autoUpdate(ref, tooltip, updatePosition);
					}
					return;
				}

				if (motion === 'slide') {
					tooltip.classList.remove(
						options.slide === 'left' ? 'translate-x-full' : 'translate-y-full'
					);
					tooltip.classList.add(options.slide === 'left' ? 'translate-x-0' : 'translate-y-0');
				} else {
					tooltip.classList.remove('hidden');
				}
				tooltip.classList.remove('opacity-0');

				if (!shouldSkipAutoPosition()) {
					await updatePosition();
					if (gen !== visibilityLayoutGen) return;
					close = autoUpdate(ref, tooltip, updatePosition);
				}
			});
		} else {
			close?.();
			if (motion === 'slide') {
				tooltip.classList.add(options.slide === 'left' ? 'translate-x-full' : 'translate-y-full');
			} else if (motion === 'daisy') {
				tooltip.classList.add('tooltip-portal-daisy-host--inactive', 'opacity-0');
			} else {
				tooltip.classList.add('hidden');
			}
			if (motion !== 'daisy') {
				tooltip.classList.add('opacity-0');
			}
			close = null;
		}
	});

	function shouldSkipAutoPosition(): boolean {
		// Menu passes fixed: true for manually laid-out dropdowns; still run when using Floating UI fixed strategy.
		return !!(options?.fixed && options?.strategy !== 'fixed');
	}

	async function updatePosition() {
		if (!ref || !tooltip || shouldSkipAutoPosition()) return;

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
			checkReady();
		},
		tooltip: (node: HTMLElement, params?: TooltipOptions) => {
			tooltip = node;
			// Create a new object to ensure reactivity
			options = {
				...initialOptions,
				...params
			};
			checkReady();

			return {
				update(newParams?: TooltipOptions) {
					if (newParams) {
						const merged = { ...options, ...newParams };
						if (tooltipOverlayUnchanged(options, merged)) {
							return;
						}
						options = merged;
					}
				},
				destroy() {
					// Clean up the tooltip if it's in document.body
					if (tooltip && tooltip.parentElement === document.body) {
						tooltip.remove();
					}
				}
			};
		},
		toggle: (newOpenValue?: boolean) => {
			const willBeOpen = newOpenValue ?? !open;
			// Only dispatch toolOpen if we're actually opening (going from closed to open)
			if (!open && willBeOpen && !options?.hover) {
				document.dispatchEvent(new CustomEvent('toolOpen', { detail: selfId.toString() }));
			}
			open = willBeOpen;
			options?.onOpenChange?.(open);
		}
	};
}
