export function autoHeight(node: HTMLTextAreaElement) {
	const scrollContainer = findScrollContainer(node);
	let lastValue = node.value;

	const resize = () => {
		if (node.value === lastValue) return;

		lastValue = node.value;
		const prevScrollTop = scrollContainer?.scrollTop ?? 0;
		const { selectionStart, selectionEnd } = node;

		node.style.height = 'auto';
		node.style.height = node.scrollHeight + 'px';

		requestAnimationFrame(() => {
			if (selectionStart != null && selectionEnd != null) {
				node.setSelectionRange(selectionStart, selectionEnd);
			}
			if (scrollContainer) {
				scrollContainer.scrollTop = prevScrollTop;
			}
		});
	};

	if ('fieldSizing' in node.style) {
		node.style.fieldSizing = node.value === '' ? 'fixed' : 'content';
	}
	node.classList.add('scrollbar-none');

	node.oninput = node.onfocus = node.onchange = node.onkeyup = () => resize();

	const resizeObserver = new ResizeObserver(() => {
		requestAnimationFrame(() => resize());
	});
	resizeObserver.observe(node.parentElement!);

	let animationFrame = requestAnimationFrame(() => resize());

	const mutationObserver = new MutationObserver(() => {
		cancelAnimationFrame(animationFrame);
		animationFrame = requestAnimationFrame(() => resize());
	});
	mutationObserver.observe(node, { characterData: true, subtree: true });

	return {
		destroy() {
			resizeObserver.disconnect();
			mutationObserver.disconnect();
		}
	};
}

function findScrollContainer(el: HTMLElement): HTMLElement | null {
	let parent = el.parentElement;
	while (parent) {
		const style = getComputedStyle(parent);
		if (/(auto|scroll)/.test(style.overflowY)) {
			return parent;
		}
		parent = parent.parentElement;
	}
	return null;
}
