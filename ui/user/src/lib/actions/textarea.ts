export function resize(node: HTMLTextAreaElement) {
	const currentHeight = node.style.height;
	const currentMinHeight = node.style.minHeight;

	const overflow = node.style.overflow;
	node.style.overflow = 'hidden';

	node.style.height = 'auto';
	const scrollHeight = node.scrollHeight;

	node.style.height = currentHeight;
	node.style.overflow = overflow;

	// Only update if the height actually changed
	const newMinHeight = scrollHeight + 'px';
	if (newMinHeight !== currentMinHeight) {
		node.style.minHeight = newMinHeight;
	}
}

export function autoHeight(node: HTMLTextAreaElement) {
	if ('fieldSizing' in node.style) {
		if (node.value === '') {
			// This is so that rows=2 works
			node.style.fieldSizing = 'fixed';
		} else {
			node.style.fieldSizing = 'content';
		}
	}
	node.classList.add('scrollbar-none');
	node.onkeyup = () => resize(node);
	node.onfocus = () => resize(node);
	node.oninput = () => resize(node);
	node.onresize = () => resize(node);
	node.onchange = () => resize(node);

	// Add resize observer to handle container resizing
	const resizeObserver = new ResizeObserver(() => {
		// Debounce the resize calculation
		requestAnimationFrame(() => {
			resize(node);
		});
	});

	resizeObserver.observe(node.parentElement!);

	// Clean up observer when element is destroyed
	return {
		destroy() {
			resizeObserver.disconnect();
		}
	};
}
