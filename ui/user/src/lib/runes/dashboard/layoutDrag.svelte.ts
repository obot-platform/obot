import {
	createDragAnchor,
	createDragSession,
	floatDragClone,
	scrollableAncestor
} from '$lib/services/dashboard/dragScroll';
import {
	MAX_WIDGETS_PER_ROW,
	insertBox,
	insertHeader,
	moveBox,
	moveHeader,
	placeWidget,
	rowAt,
	type DashboardConfig,
	type WidgetId
} from '$lib/services/dashboard/layout';
import { onDestroy } from 'svelte';

export type LayoutDragSubject =
	| { kind: 'widget'; id: WidgetId }
	| { kind: 'header'; id: string }
	| { kind: 'new-header' }
	| { kind: 'box'; id: string }
	| { kind: 'new-box' };

export interface LayoutDragOptions {
	/** The layout being edited; drops are resolved against it. */
	editing: () => DashboardConfig;
	/** Stages the layout a drop produces. */
	stage: (next: DashboardConfig) => void;
	/** The scrolling layout list, so pointer travel accounts for its scroll. */
	scroller: () => HTMLElement | undefined;
}

export type LayoutDrag = ReturnType<typeof createLayoutDrag>;

export function locationKey(path: number[]): string {
	return path.join('-');
}

/**
 * Pointer drag in the Configure Dashboard layout list: widgets, headers, and boxes, from the list
 * or from the palette. Drop targets are found through data-layout-* and data-drop-slot attributes;
 * the highlighted target shows as a settled preview (see gapGhost / slotGhost) and the drop stages
 * the result. Must be created during component initialization.
 */
export function createLayoutDrag(options: LayoutDragOptions) {
	let dragging = $state<LayoutDragSubject | null>(null);
	let dropKey = $state<string | null>(null);
	let dragPreview: HTMLElement | null = null;
	let pointerX = 0;
	let pointerY = 0;

	type PendingDrop =
		| { kind: 'slot'; parent: number[]; rowIndex: number; index: number }
		| { kind: 'gap'; parent: number[]; index: number };

	let pendingDrop: PendingDrop | null = null;
	let ghostReady = $state(false);
	const SETTLED_MS = 160;
	const dropHoldPx = 8;

	function clearDragPreview() {
		dragPreview?.remove();
		dragPreview = null;
	}

	function setGrabbingCursor(active: boolean) {
		if (typeof document === 'undefined') return;
		document.documentElement.classList.toggle('dashboard-dragging', active);
	}

	function clearPendingDrop() {
		pendingDrop = null;
		dropKey = null;
		syncSettledPreview();
	}

	function dropStorageKey(next: PendingDrop): string {
		return next.kind === 'gap'
			? `gap-${locationKey([...next.parent, next.index])}`
			: `slot-${locationKey([...next.parent, next.rowIndex])}-${next.index}`;
	}

	function layoutScrollTop(): number {
		let total = 0;
		let current: HTMLElement | null = options.scroller() ?? null;
		while (current) {
			total += current.scrollTop;
			current = current.parentElement;
		}
		return total;
	}

	const dropAnchor = createDragAnchor(layoutScrollTop);
	const drag = createDragSession({
		scroller: (x, y) =>
			typeof document === 'undefined' ? null : scrollableAncestor(document.elementFromPoint(x, y)),
		onMove: (x, y) => {
			moveDragPreview(x, y);
			highlightAt(x, y);
		},
		onDrop: (x, y) => {
			highlightAt(x, y);
			applyPendingDrop();
			endDrag();
		},
		onCancel: () => endDrag()
	});

	function rememberAnchor() {
		dropAnchor.remember(pointerX, pointerY);
	}

	function dropTravel(): number {
		return dropAnchor.travel(pointerX, pointerY);
	}

	function boxWithMargin(node: HTMLElement): DOMRect {
		const rect = node.getBoundingClientRect();
		const style = getComputedStyle(node);
		const top = Number.parseFloat(style.marginTop) || 0;
		const right = Number.parseFloat(style.marginRight) || 0;
		const bottom = Number.parseFloat(style.marginBottom) || 0;
		const left = Number.parseFloat(style.marginLeft) || 0;
		return new DOMRect(
			rect.left - left,
			rect.top - top,
			rect.width + left + right,
			rect.height + top + bottom
		);
	}

	function rectContains(rect: DOMRect, x: number, y: number, pad = 0): boolean {
		return (
			x >= rect.left - pad && x <= rect.right + pad && y >= rect.top - pad && y <= rect.bottom + pad
		);
	}

	function stillInsideCurrentDrop(x: number, y: number): boolean {
		if (!dropKey || typeof document === 'undefined') return false;
		const id = dropKey.slice(dropKey.indexOf('-') + 1);
		const slot = document.querySelector(`[data-drop-slot="${CSS.escape(id)}"]`);
		if (slot instanceof HTMLElement && rectContains(boxWithMargin(slot), x, y, 4)) return true;
		for (const ghost of document.querySelectorAll('[data-drag-settled]')) {
			if (ghost instanceof HTMLElement && rectContains(boxWithMargin(ghost), x, y, 4)) return true;
		}
		return false;
	}

	function rememberDrop(next: PendingDrop) {
		const key = dropStorageKey(next);
		if (key === dropKey) return;
		pendingDrop = next;
		dropKey = key;
		rememberAnchor();
		syncSettledPreview();
	}

	function settleClone(settled: boolean) {
		if (!dragPreview) return;
		dragPreview.style.transition = `transform ${SETTLED_MS}ms ease, opacity ${SETTLED_MS}ms ease`;
		dragPreview.style.transformOrigin = 'center';
		dragPreview.style.transform = settled ? 'scale(0)' : 'scale(1)';
		dragPreview.style.opacity = settled ? '0' : '1';
	}

	function syncSettledPreview() {
		if (typeof window === 'undefined') return;
		if (!dragging || !dropKey) {
			ghostReady = false;
			if (dragging) {
				settleClone(false);
				moveDragPreview(pointerX, pointerY);
			}
			return;
		}
		settleClone(true);
		ghostReady = true;
	}

	function gapGhost(key: string) {
		return ghostReady && dropKey === `gap-${key}`;
	}

	function slotGhost(key: string) {
		return ghostReady && dropKey === `slot-${key}`;
	}

	function moveDragPreview(x: number, y: number) {
		if (!dragPreview) return;
		dragPreview.style.left = `${x - 14}px`;
		dragPreview.style.top = `${y - (dragPreview.offsetHeight / 2 || 12)}px`;
	}

	function mountDragPreview(source: HTMLElement, x: number, y: number) {
		const origin = source.closest('[data-drag-preview]');
		if (!(origin instanceof HTMLElement)) return;
		const preview = origin.cloneNode(true);
		if (!(preview instanceof HTMLElement)) return;

		clearDragPreview();
		floatDragClone(preview, { zIndex: '10000', maxWidth: 'none', opacity: '1' });
		preview.style.visibility = 'visible';
		preview.style.transformOrigin = 'center';
		preview.style.transform = 'scale(1)';
		preview.classList.remove('min-w-0', 'flex-1');
		preview.classList.add(
			'border-base-300',
			'bg-base-200',
			'rounded-lg',
			'border',
			'px-2',
			'py-1',
			'shadow-lg'
		);
		preview.querySelector('span')?.classList.remove('truncate');
		preview.querySelectorAll('button').forEach((button) => button.remove());
		const layer = source.closest('dialog') ?? document.body;
		layer.appendChild(preview);
		dragPreview = preview;
		moveDragPreview(x, y);
	}

	function beginDrag(event: PointerEvent, subject: LayoutDragSubject) {
		if (event.button !== 0) return;
		const current = event.currentTarget;
		if (!(current instanceof HTMLElement)) return;
		const target = event.target;
		if (target instanceof Element) {
			const interactive = target.closest('button, input, textarea, a, label');
			if (interactive && interactive !== current) return;
		}
		event.preventDefault();
		dragging = subject;
		pointerX = event.clientX;
		pointerY = event.clientY;
		drag.start(event.clientX, event.clientY);
		clearPendingDrop();
		rememberAnchor();
		setGrabbingCursor(true);
		mountDragPreview(current, event.clientX, event.clientY);
	}

	function endDrag() {
		dragging = null;
		ghostReady = false;
		clearPendingDrop();
		clearDragPreview();
		setGrabbingCursor(false);
		drag.stop();
	}

	function rowAccepts(parent: number[], rowIndex: number) {
		const subject = dragging;
		const row = rowAt(options.editing().rows, [...parent, rowIndex]);
		if (!row || row.kind !== 'widgets' || !subject || subject.kind !== 'widget') return false;
		return (
			row.items.some((placement) => placement.id === subject.id) ||
			row.items.length < MAX_WIDGETS_PER_ROW
		);
	}

	function insertionIndex(row: HTMLElement, clientX: number): number {
		const cards = row.querySelectorAll('[data-layout-card]');
		for (let index = 0; index < cards.length; index += 1) {
			const card = cards[index];
			if (!(card instanceof HTMLElement)) continue;
			const rect = card.getBoundingClientRect();
			if (clientX < rect.left + rect.width / 2) return index;
		}
		return cards.length;
	}

	function draggingSingleton() {
		return (
			dragging?.kind === 'header' ||
			dragging?.kind === 'new-header' ||
			dragging?.kind === 'box' ||
			dragging?.kind === 'new-box'
		);
	}

	function singletonAt(parent: number[], index: number): DashboardConfig | undefined {
		if (dragging?.kind === 'new-header') return insertHeader(options.editing(), index, '', parent);
		if (dragging?.kind === 'header')
			return moveHeader(options.editing(), dragging.id, index, parent);
		if (dragging?.kind === 'new-box') return insertBox(options.editing(), index, parent);
		if (dragging?.kind === 'box') return moveBox(options.editing(), dragging.id, index, parent);
		return undefined;
	}

	function locationFrom(key: string | null): { parent: number[]; index: number } | null {
		if (!key) return null;
		const parts = key.split('-').map((part) => Number(part));
		if (parts.length === 0 || parts.some((part) => !Number.isInteger(part))) return null;
		const index = parts[parts.length - 1];
		return { parent: parts.slice(0, -1), index };
	}

	function rememberGap(parent: number[], index: number) {
		rememberDrop({ kind: 'gap', parent, index });
	}

	function rowEdge(rect: DOMRect, y: number): 'before' | 'after' | null {
		const band = Math.min(24, Math.max(14, rect.height * 0.22));
		if (y <= rect.top + band) return 'before';
		if (y >= rect.bottom - band) return 'after';
		return null;
	}

	function dropOnRow(row: HTMLElement, x: number, y: number) {
		const location = locationFrom(row.getAttribute('data-layout-row'));
		if (!location) {
			clearPendingDrop();
			return;
		}
		const rect = row.getBoundingClientRect();
		const edge = draggingSingleton()
			? y < rect.top + rect.height / 2
				? 'before'
				: 'after'
			: rowEdge(rect, y);
		if (edge) {
			rememberGap(location.parent, edge === 'before' ? location.index : location.index + 1);
			return;
		}
		if (!rowAccepts(location.parent, location.index)) {
			clearPendingDrop();
			return;
		}
		rememberDrop({
			kind: 'slot',
			parent: location.parent,
			rowIndex: location.index,
			index: insertionIndex(row, x)
		});
	}

	function dropOnHeader(header: HTMLElement, y: number) {
		const location = locationFrom(header.getAttribute('data-layout-location'));
		if (!location) {
			clearPendingDrop();
			return;
		}
		const rect = header.getBoundingClientRect();
		const index = y < rect.top + rect.height / 2 ? location.index : location.index + 1;
		rememberGap(location.parent, index);
	}

	function dropOnBox(box: HTMLElement, x: number, y: number) {
		const location = locationFrom(box.getAttribute('data-box-location'));
		if (!location) {
			clearPendingDrop();
			return;
		}
		const titleBottom =
			box.firstElementChild instanceof HTMLElement
				? box.firstElementChild.getBoundingClientRect().bottom
				: box.getBoundingClientRect().top;
		const contentBottom =
			box.lastElementChild instanceof HTMLElement
				? box.lastElementChild.getBoundingClientRect().bottom
				: titleBottom;
		if (y <= titleBottom) {
			rememberGap(location.parent, location.index);
			return;
		}
		if (y >= contentBottom) {
			rememberGap(
				[...location.parent, location.index],
				Number(box.getAttribute('data-child-count') ?? '0')
			);
			return;
		}
		const inner = [
			...box.querySelectorAll('[data-layout-row], [data-layout-header], [data-layout-box]')
		]
			.filter((node) => node.parentElement?.closest('[data-layout-box]') === box)
			.find((node) => {
				const rect = node.getBoundingClientRect();
				return y >= rect.top && y <= rect.bottom;
			});
		if (inner instanceof HTMLElement && inner.hasAttribute('data-layout-row')) {
			dropOnRow(inner, x, y);
			return;
		}
		if (inner instanceof HTMLElement && inner.hasAttribute('data-layout-header')) {
			dropOnHeader(inner, y);
			return;
		}
		if (inner instanceof HTMLElement && inner.hasAttribute('data-layout-box')) {
			dropOnBox(inner, x, y);
			return;
		}
		clearPendingDrop();
	}

	function highlightAt(x: number, y: number) {
		if (typeof document === 'undefined') return;
		pointerX = x;
		pointerY = y;
		if (dropKey && (stillInsideCurrentDrop(x, y) || dropTravel() < dropHoldPx)) return;
		const node = document.elementFromPoint(x, y);
		const target = node instanceof Element ? node : null;
		const gap = target?.closest('[data-layout-gap]');
		if (gap instanceof HTMLElement) {
			const location = locationFrom(gap.getAttribute('data-layout-gap'));
			if (location) rememberGap(location.parent, location.index);
			else clearPendingDrop();
			return;
		}
		const row = target?.closest('[data-layout-row]');
		if (row instanceof HTMLElement) {
			dropOnRow(row, x, y);
			return;
		}
		const header = target?.closest('[data-layout-header]');
		if (header instanceof HTMLElement) {
			dropOnHeader(header, y);
			return;
		}
		const box = target?.closest('[data-layout-box]');
		if (box instanceof HTMLElement) {
			dropOnBox(box, x, y);
			return;
		}
		clearPendingDrop();
	}

	function applyPendingDrop() {
		const drop = pendingDrop;
		if (!drop || !dragging) return;
		if (drop.kind === 'gap') {
			if (dragging.kind === 'widget') {
				options.stage(
					placeWidget(options.editing(), dragging.id, {
						kind: 'new-row',
						path: drop.parent,
						index: drop.index
					})
				);
				return;
			}
			const next = singletonAt(drop.parent, drop.index);
			if (next) options.stage(next);
			return;
		}
		if (dragging.kind !== 'widget' || !rowAccepts(drop.parent, drop.rowIndex)) return;
		options.stage(
			placeWidget(options.editing(), dragging.id, {
				kind: 'slot',
				path: [...drop.parent, drop.rowIndex],
				index: drop.index
			})
		);
	}

	onDestroy(endDrag);

	return {
		get dragging() {
			return dragging;
		},
		/** `gap-<location>` or `slot-<row location>-<index>` of the highlighted drop, if any. */
		get dropKey() {
			return dropKey;
		},
		/** Whether the settled preview shows in the gap before row `key`. */
		gapGhost,
		/** Whether the settled preview shows in widget slot `key`. */
		slotGhost,
		begin: beginDrag,
		end: endDrag
	};
}
