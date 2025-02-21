import { tick } from 'svelte';

export function sticktobottom(node: HTMLElement) {
	const observer = new MutationObserver(() => {
		if (node.dataset.autoscroll !== 'false') {
			tick().then(() => {
				node.scrollTop = node.scrollHeight - node.clientHeight;
			});
		}
	});

	observer.observe(node, { childList: true, subtree: true });

	node.addEventListener('wheel', (e) => {
		if (e.deltaY < 0) {
			node.dataset.autoscroll = 'false';
			node.dataset.shouldRecalculate = 'false';
		} else {
			node.dataset.shouldRecalculate = 'true';
		}
	});

	node.addEventListener('scroll', () => {
		// Calculate the scroll position
		const scrollTop = node.scrollTop;
		const scrollHeight = node.scrollHeight;
		const clientHeight = node.clientHeight;
		const isAtBottom = scrollHeight - scrollTop - clientHeight <= 40;

		if (node.dataset.shouldRecalculate === 'false') {
			return;
		}

		if (isAtBottom) {
			node.dataset.autoscroll = 'true';
			node.dataset.shouldRecalculate = 'false';
		} else {
			node.dataset.autoscroll = 'false';
		}
	});
}
