import popover from './popover.svelte.js';

function hasOverflow(element: HTMLElement) {
	return element.scrollHeight > element.clientHeight || element.scrollWidth > element.clientWidth;
}

export function overflowToolTip(node: HTMLElement) {
	const { ref, tooltip } = popover({
		placement: 'top-end',
		offset: 10
	});

	node.classList.add('truncate');

	const p = document.createElement('p') as HTMLParagraphElement;
	p.classList.add('tooltip');
	p.textContent = node.textContent;

	node.insertAdjacentElement('afterend', p);
	node.addEventListener('mouseenter', (e) => {
		if (!hasOverflow(node)) {
			e.stopImmediatePropagation();
		}
		// Update content if changed
		p.textContent = node.textContent;
	});

	// Register after the above event listener to ensure we can stop propagation
	tooltip(p, { hover: true });
	ref(node);
}
