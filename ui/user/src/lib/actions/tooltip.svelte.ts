import { autoUpdate, computePosition, flip, offset, shift, type Placement } from '@floating-ui/dom';
import { tick } from 'svelte';
import type { Action } from 'svelte/action';

export type TooltipActionOptions = {
	disabled?: boolean;
	anchor?: HTMLElement;
	placement?: Placement;
	offset?: number;
	delay?: number;
};

export const tooltip: Action<HTMLElement, TooltipActionOptions> = (node, options) => {
	node.classList.add('hidden', 'absolute', 'transition-opacity', 'duration-300', 'opacity-0');

	const hasZIndex = Array.from(node.classList).some((className) => className.startsWith('z-'));

	if (!hasZIndex) {
		node.classList.add('z-30');
	}

	$effect(() => {
		const anchorEl = options.anchor;

		if (!anchorEl) return;

		let close: (() => void) | undefined;
		let timeout: number;

		const handleOpen = () => {
			if (!anchorEl || options.disabled) return;

			timeout = setTimeout(() => {
				close = showTooltip();
			}, options.delay ?? 0);
		};

		const handleClose = () => {
			clearTimeout(timeout);
			close?.();
		};

		anchorEl.addEventListener('mouseenter', handleOpen);
		anchorEl.addEventListener('mouseleave', handleClose);

		return () => {
			anchorEl.removeEventListener('mouseenter', handleOpen);
			anchorEl.removeEventListener('mouseleave', handleClose);
			handleClose();
		};
	});

	return {
		update(newOpts) {
			options = { ...options, ...newOpts };
		}
	};

	async function updatePosition() {
		if (!options.anchor) return;

		const offsetVal = options.offset ?? 2;

		const { x, y } = await computePosition(options.anchor, node, {
			placement: options.placement,
			middleware: [flip(), shift({ padding: offsetVal }), offset(offsetVal)]
		});

		Object.assign(node.style, {
			left: `${x}px`,
			top: `${y}px`
		});
	}

	function showTooltip() {
		if (!options.anchor) return;

		node.classList.remove('hidden');
		tick().then(() => {
			node.classList.remove('opacity-0');
		});
		updatePosition();
		const close = autoUpdate(options.anchor, node, updatePosition);

		return () => {
			close();
			node.classList.add('hidden', 'opacity-0');
		};
	}
};
