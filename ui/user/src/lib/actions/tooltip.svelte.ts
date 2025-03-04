import { autoUpdate, computePosition, flip, offset, shift, type Placement } from '@floating-ui/dom';
import { tick } from 'svelte';
import type { Action } from 'svelte/action';

export type TooltipActionOptions = {
	disabled?: () => boolean;
	placement?: Placement;
	offset?: number;
	delay?: number;
};

export const createTooltip = (opts?: TooltipActionOptions) => {
	let anchorRef = $state<HTMLElement | null>(null);
	let contentRef = $state<HTMLElement | null>(null);

	const options = $state<TooltipActionOptions>({
		placement: 'top',
		offset: 2,
		delay: 0,
		...opts
	});

	$effect(() => {
		contentRef?.classList.add(
			'hidden',
			'absolute',
			'transition-opacity',
			'duration-300',
			'opacity-0'
		);
	});

	const build = () => {
		if (!anchorRef || !contentRef) return;

		let close: (() => void) | undefined;
		let timeout: number;

		const handleOpen = () => {
			if (!anchorRef || !contentRef || options.disabled?.()) return;

			timeout = setTimeout(() => {
				close = showTooltip();
			}, options.delay ?? 0);
		};

		const handleClose = () => {
			clearTimeout(timeout);
			close?.();
		};

		anchorRef.addEventListener('mouseenter', handleOpen);
		anchorRef.addEventListener('mouseleave', handleClose);

		return () => {
			anchorRef?.removeEventListener('mouseenter', handleOpen);
			anchorRef?.removeEventListener('mouseleave', handleClose);
			handleClose();
		};
	};

	const anchor: Action<HTMLElement> = (node) => {
		anchorRef = node;

		const cleanup = build();

		return {
			destroy() {
				cleanup?.();
				anchorRef = null;
			}
		};
	};

	const content: Action<HTMLElement> = (node) => {
		contentRef = node;
		const cleanup = build();

		return {
			destroy() {
				cleanup?.();
				contentRef = null;
			}
		};
	};

	return { anchor, content };

	async function updatePosition() {
		if (!anchorRef || !contentRef) return;

		const offsetVal = options.offset ?? 2;

		const { x, y } = await computePosition(anchorRef, contentRef, {
			placement: options.placement,
			middleware: [flip(), shift({ padding: offsetVal }), offset(offsetVal)]
		});

		Object.assign(contentRef.style, {
			left: `${x}px`,
			top: `${y}px`
		});
	}

	function showTooltip() {
		if (!anchorRef || !contentRef) return;

		contentRef.classList.remove('hidden');
		tick().then(() => {
			contentRef?.classList.remove('opacity-0');
		});
		updatePosition();
		const close = autoUpdate(anchorRef, contentRef, updatePosition);

		return () => {
			close();
			contentRef?.classList.add('hidden', 'opacity-0');
		};
	}
};
