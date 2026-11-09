const DRAG_SCROLL_EDGE = 64;
const DRAG_SCROLL_MAX = 16;

function contentTop(scroller: HTMLElement, top: number): number {
	let edge = top;
	for (const child of scroller.children) {
		if (!(child instanceof HTMLElement) || getComputedStyle(child).position !== 'sticky') {
			continue;
		}
		edge = Math.max(edge, child.getBoundingClientRect().bottom);
	}
	return edge;
}

function scrollStep(distance: number, room: number): number {
	const depth = Math.min(1, (DRAG_SCROLL_EDGE - distance) / DRAG_SCROLL_EDGE);
	return Math.min(room, Math.max(1, Math.round(depth * DRAG_SCROLL_MAX)));
}

export function dragScrollDelta(scroller: HTMLElement | null, y: number): number {
	if (!scroller) return 0;
	const rect = scroller.getBoundingClientRect();
	const top = Math.max(rect.top, 0);
	const visibleTop = contentTop(scroller, top);
	const bottom = Math.min(rect.bottom, window.innerHeight);
	const above = scroller.scrollTop;
	const below = scroller.scrollHeight - scroller.clientHeight - scroller.scrollTop;
	const fromTop = y - visibleTop;
	const fromBottom = bottom - y;
	const upward =
		above > 1 && fromTop >= top - visibleTop - 8 && fromTop < DRAG_SCROLL_EDGE
			? -scrollStep(fromTop, above)
			: 0;
	const downward =
		below > 1 && fromBottom >= -8 && fromBottom < DRAG_SCROLL_EDGE
			? scrollStep(fromBottom, below)
			: 0;
	if (upward && downward) return fromTop <= fromBottom ? upward : downward;
	return upward || downward;
}
