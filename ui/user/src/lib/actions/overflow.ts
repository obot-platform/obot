import popover from './popover.svelte.js';

function hasOverflow(element: HTMLElement) {
	return element.scrollHeight > element.clientHeight || element.scrollWidth > element.clientWidth;
}

export function overflowToolTip(node: HTMLElement) {
	const { ref, tooltip } = popover({
		placement: 'top',
		offset: 4,
		hover: true
	});

	node.classList.add('text-nowrap', 'overflow-hidden', 'text-ellipsis', 'w-full');

	const span = document.createElement('p') as HTMLSpanElement;
	span.classList.add('tooltip');
	span.textContent = node.textContent;

	node.insertAdjacentElement('afterend', span);
	node.addEventListener('mouseenter', (e) => {
		if (!hasOverflow(node)) {
			e.stopImmediatePropagation();
		}
	});

	// Register after the above event listener to ensure we can stop propagation
	tooltip(span);
	ref(node);
}
