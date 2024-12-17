import { tick } from 'svelte';

export function autoscroll(node: HTMLElement) {
	const observer = new MutationObserver(() => {
		if (node.dataset.scroll !== 'false') {
			tick().then(() => {
				node.scrollTop = node.scrollHeight;
			});
		}
	});

	observer.observe(node, { childList: true, subtree: true });

	node.addEventListener('scroll', () => {
		// Calculate the scroll position
		const scrollTop = node.scrollTop;
		const scrollHeight = node.scrollHeight;
		const clientHeight = node.clientHeight;

		// Check if the user has scrolled to the bottom
		if (scrollTop + clientHeight >= scrollHeight) {
			node.dataset.scroll = 'true';
		} else {
			node.dataset.scroll = 'false';
		}
	});
}
