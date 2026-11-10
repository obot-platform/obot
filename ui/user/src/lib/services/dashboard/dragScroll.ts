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

export interface DragAnchor {
	remember(x: number, y: number): void;
	travel(x: number, y: number): number;
}

export function createDragAnchor(scrollTop: () => number): DragAnchor {
	let anchorX = 0;
	let anchorY = 0;
	let anchorScroll = 0;
	return {
		remember(x, y) {
			anchorX = x;
			anchorY = y;
			anchorScroll = scrollTop();
		},
		travel(x, y) {
			return Math.hypot(x - anchorX, y - anchorY + (scrollTop() - anchorScroll));
		}
	};
}

/** Nearest element, starting at `from`, that scrolls vertically and has overflow to scroll. */
export function scrollableAncestor(from: Element | null | undefined): HTMLElement | null {
	let current = from ?? null;
	while (current instanceof HTMLElement) {
		const overflowY = getComputedStyle(current).overflowY;
		if (
			(overflowY === 'auto' || overflowY === 'scroll') &&
			current.scrollHeight > current.clientHeight + 1
		) {
			return current;
		}
		current = current.parentElement;
	}
	return null;
}

export function floatDragClone(
	clone: HTMLElement,
	style: { zIndex: string; maxWidth: string; opacity: string }
) {
	clone.setAttribute('aria-hidden', 'true');
	clone.setAttribute('data-drag-clone', '');
	clone.style.position = 'fixed';
	clone.style.margin = '0';
	clone.style.width = 'max-content';
	clone.style.maxWidth = style.maxWidth;
	clone.style.opacity = style.opacity;
	clone.style.pointerEvents = 'none';
	clone.style.zIndex = style.zIndex;
}

export interface DragSessionOptions {
	/** The element to auto-scroll while the pointer is near its top or bottom edge. */
	scroller: (x: number, y: number) => HTMLElement | null;
	/** Runs after each pointer move and after each auto-scroll step. */
	onMove: (x: number, y: number) => void;
	/** Runs on pointerup with its coordinates; x and y keep the last pointermove position. */
	onDrop: (x: number, y: number) => void;
	/** Runs on pointercancel or Escape. */
	onCancel: () => void;
}

export interface DragSession {
	readonly active: boolean;
	readonly x: number;
	readonly y: number;
	start(x: number, y: number): void;
	stop(): void;
}

export function createDragSession(options: DragSessionOptions): DragSession {
	let active = false;
	let x = 0;
	let y = 0;
	let frame = 0;

	function stopScroll() {
		if (!frame || typeof cancelAnimationFrame === 'undefined') return;
		cancelAnimationFrame(frame);
		frame = 0;
	}

	function tick() {
		frame = 0;
		if (!active) return;
		const scroller = options.scroller(x, y);
		const delta = dragScrollDelta(scroller, y);
		if (!delta || !scroller) return;
		scroller.scrollTop += delta;
		options.onMove(x, y);
		frame = requestAnimationFrame(tick);
	}

	function syncScroll() {
		if (!dragScrollDelta(options.scroller(x, y), y)) {
			stopScroll();
			return;
		}
		if (!frame) frame = requestAnimationFrame(tick);
	}

	function onPointerMove(event: PointerEvent) {
		if (!active) return;
		x = event.clientX;
		y = event.clientY;
		options.onMove(x, y);
		syncScroll();
	}

	function onPointerUp(event: PointerEvent) {
		if (!active) return;
		options.onDrop(event.clientX, event.clientY);
	}

	function onPointerCancel() {
		if (active) options.onCancel();
	}

	function onKeyDown(event: KeyboardEvent) {
		if (!active || event.key !== 'Escape') return;
		event.preventDefault();
		options.onCancel();
	}

	function stop() {
		active = false;
		stopScroll();
		if (typeof window === 'undefined') return;
		window.removeEventListener('pointermove', onPointerMove);
		window.removeEventListener('pointerup', onPointerUp);
		window.removeEventListener('pointercancel', onPointerCancel);
		window.removeEventListener('keydown', onKeyDown);
	}

	return {
		get active() {
			return active;
		},
		get x() {
			return x;
		},
		get y() {
			return y;
		},
		start(startX, startY) {
			stop();
			active = true;
			x = startX;
			y = startY;
			window.addEventListener('pointermove', onPointerMove);
			window.addEventListener('pointerup', onPointerUp);
			window.addEventListener('pointercancel', onPointerCancel);
			window.addEventListener('keydown', onKeyDown);
		},
		stop
	};
}
