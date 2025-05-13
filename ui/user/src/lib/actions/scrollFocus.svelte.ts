/**
 * Scrolls to the last added child element on the DOM if it is out of view.
 * Add onto a scrollable parent element
 */
export function scrollFocus(node: HTMLElement) {
	function isOutOfView(element: Element): boolean {
		const rect = element.getBoundingClientRect();
		const containerRect = node.getBoundingClientRect();

		const visibleHeight =
			Math.min(rect.bottom, containerRect.bottom) - Math.max(rect.top, containerRect.top);
		const elementHeight = rect.height;

		// If less than 10% is visible, consider it out of view
		return visibleHeight < elementHeight * 0.1;
	}

	function handleMutations(mutations: MutationRecord[]) {
		const addedNodes = mutations.flatMap((m) => Array.from(m.addedNodes));
		if (addedNodes.length === 0) return;

		const lastAddedElement = addedNodes
			.filter((node): node is Element => {
				if (!(node instanceof Element)) return false;
				// want to avoid scrolling nested dialog content
				return !node.closest('dialog');
			})
			.pop();

		if (lastAddedElement && isOutOfView(lastAddedElement)) {
			const startTime = Date.now();
			const duration = 350;
			const element = lastAddedElement;

			// this is to ensure continuous scroll update happens during a slide
			function updateScroll() {
				const elapsed = Date.now() - startTime;
				if (elapsed < duration) {
					requestAnimationFrame(() => {
						element.scrollIntoView({ behavior: 'auto', block: 'end' });
						updateScroll();
					});
				}
			}

			updateScroll();
		}
	}

	const observer = new MutationObserver(handleMutations);

	observer.observe(node, {
		childList: true,
		subtree: true
	});

	return {
		destroy() {
			observer.disconnect();
		}
	};
}
