import popover from './popover.svelte.js';
import type { Placement } from '@floating-ui/dom';
import { twMerge } from 'tailwind-merge';

function hasOverflow(element: HTMLElement) {
	return element.scrollHeight > element.clientHeight || element.scrollWidth > element.clientWidth;
}

type OverflowTooltipOptions = {
	placement?: Placement;
	tooltipClass?: string;
	offset?: number;
};

export function overflowToolTip(
	n: HTMLElement,
	{ placement = 'top', tooltipClass, offset = 4 }: OverflowTooltipOptions = {}
) {
	const { ref, tooltip } = popover({ placement, offset, hover: true });

	let node = n;

	// this is a crappy workaround to make line-clamp work on elements that don't specify a line height by default
	if (!['p', 'span'].includes(n.tagName)) {
		const span = document.createElement('span');
		span.textContent = n.textContent;
		n.textContent = '';
		n.appendChild(span);
		node = span;
	}

	if (!getComputedStyle(node).lineHeight) {
		node.classList.add('leading-2');
	}

	node.classList.add('line-clamp-1', 'break-all');

	const p = document.createElement('p');
	p.classList.add(...twMerge('tooltip break-all', tooltipClass).split(' '));
	p.textContent = node.textContent;

	node.insertAdjacentElement('afterend', p);
	node.addEventListener('mouseenter', (e) => {
		if (!hasOverflow(node)) {
			e.stopImmediatePropagation();
		}
	});

	// Register after the above event listener to ensure we can stop propagation
	tooltip(p);
	ref(node);
}
