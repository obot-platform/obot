/** Find nearest ancestor that scrolls vertically. */
export function findScrollContainer(element: HTMLElement): HTMLElement | null {
	let parent: HTMLElement | null = element.parentElement;
	while (parent) {
		const style = getComputedStyle(parent);
		if (style.overflowY === 'auto' || style.overflowY === 'scroll') {
			return parent;
		}
		parent = parent.parentElement;
	}
	return null;
}

/**
 * Offset for a sticky table header so it sits below other sticky chrome
 * (e.g. Layout navbar) within the same scroll container.
 */
export function calculateStickyTop(wrapper: HTMLElement): number {
	const scrollContainer = findScrollContainer(wrapper);
	if (!scrollContainer) return 0;

	let maxStickyBottom = 0;
	let current: HTMLElement | null = wrapper;

	while (current && current !== scrollContainer) {
		const parent: HTMLElement | null = current.parentElement;
		if (!parent) break;

		for (const sibling of parent.children) {
			if (sibling === current) break;

			const siblingStyle = getComputedStyle(sibling);
			if (siblingStyle.position === 'sticky') {
				const top = parseFloat(siblingStyle.top) || 0;
				if (top >= 0 && top < 200) {
					maxStickyBottom = Math.max(maxStickyBottom, top + (sibling as HTMLElement).offsetHeight);
				}
			}

			for (const desc of sibling.querySelectorAll('*')) {
				const el = desc as HTMLElement;
				const otherTableRoot = el.closest('[data-table-root]');
				if (otherTableRoot && otherTableRoot !== wrapper) continue;

				const descStyle = getComputedStyle(desc);
				if (descStyle.position === 'sticky') {
					const top = parseFloat(descStyle.top) || 0;
					if (top >= 0 && top < 200) {
						maxStickyBottom = Math.max(maxStickyBottom, top + el.offsetHeight);
					}
				}
			}
		}

		current = parent;
	}

	return maxStickyBottom;
}
